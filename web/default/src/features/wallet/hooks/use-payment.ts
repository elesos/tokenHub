/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import {
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestStripePayment,
  isApiSuccess,
} from '../api'
import {
  isStripePayment,
  isWaffoPancakePayment,
  isMobileBrowser,
  openPaymentURL,
  postPaymentForm,
  resolveEpayLaunch,
} from '../lib'

// ============================================================================
// Payment Hook
// ============================================================================

export type ProcessPaymentResult =
  | { ok: false }
  | { ok: true; mode: 'redirect' }
  | { ok: true; mode: 'qrcode'; qrContent: string }

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (topupAmount: number, paymentType: string) => {
      try {
        setCalculating(true)

        const isStripe = isStripePayment(paymentType)
        const isPancake = isWaffoPancakePayment(paymentType)
        const response = isStripe
          ? await calculateStripeAmount({ amount: topupAmount })
          : isPancake
            ? await calculateWaffoPancakeAmount({ amount: topupAmount })
            : await calculateAmount({ amount: topupAmount })

        if (isApiSuccess(response) && response.data) {
          const calculatedAmount = parseFloat(response.data)
          setAmount(calculatedAmount)
          return calculatedAmount
        }

        // Don't show error for calculation, just set to 0
        setAmount(0)
        return 0
      } catch (_error) {
        setAmount(0)
        return 0
      } finally {
        setCalculating(false)
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (
      topupAmount: number,
      paymentType: string
    ): Promise<ProcessPaymentResult> => {
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)
        const amount = Math.floor(topupAmount)

        const response = isStripe
          ? await requestStripePayment({
              amount,
              payment_method: 'stripe',
            })
          : await requestPayment({
              amount,
              payment_method: paymentType,
            })

        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return { ok: false }
        }

        // Handle Stripe payment
        if (isStripe && response.data?.pay_link) {
          window.open(response.data.pay_link as string, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return { ok: true, mode: 'redirect' }
        }

        // Handle non-Stripe payment (EasyPay / form / mapi)
        if (!isStripe && response.data) {
          const url = (response as { url?: string }).url
          const payType = (response as { pay_type?: string }).pay_type
          if (!url) {
            toast.error(i18next.t('Payment request failed'))
            return { ok: false }
          }

          const launch = resolveEpayLaunch(
            url,
            response.data as Record<string, unknown>,
            payType
          )

          if (launch.type === 'qrcode') {
            toast.success(i18next.t('Please scan the QR code to pay'))
            return { ok: true, mode: 'qrcode', qrContent: launch.content }
          }

          if (launch.type === 'urlscheme') {
            if (isMobileBrowser()) {
              window.location.href = launch.url
              toast.success(i18next.t('Redirecting to payment page...'))
              return { ok: true, mode: 'redirect' }
            }
            // Desktop cannot open alipays:// / weixin:// — show QR instead of
            // the render.alipay.com intermediate page that never completes pay.
            toast.success(i18next.t('Please scan the QR code to pay'))
            return { ok: true, mode: 'qrcode', qrContent: launch.qrContent }
          }

          if (launch.type === 'form') {
            postPaymentForm(launch.url, launch.params)
          } else {
            openPaymentURL(launch.url)
          }
          toast.success(i18next.t('Redirecting to payment page...'))
          return { ok: true, mode: 'redirect' }
        }

        return { ok: false }
      } catch (_error) {
        toast.error(i18next.t('Payment request failed'))
        return { ok: false }
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return {
    amount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
  }
}
