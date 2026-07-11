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
import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { getPaymentIcon } from '../../lib'
import type { PaymentMethod } from '../../types'

interface PaymentQrDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Raw content encoded into the QR (pay URL / scheme / gateway qrcode). */
  qrContent: string
  paymentMethod?: PaymentMethod
  onPaid?: () => void
}

/**
 * Show a scannable payment QR code.
 *
 * Used when EasyPay mapi returns qrcode (or Alipay/WeChat scan links). Opening
 * those links in a desktop browser lands on intermediate pages such as
 * render.alipay.com that try to wake the native app and do not complete payment.
 */
export function PaymentQrDialog({
  open,
  onOpenChange,
  qrContent,
  paymentMethod,
  onPaid,
}: PaymentQrDialogProps) {
  const { t } = useTranslation()

  if (!qrContent) {
    return null
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-sm'>
        <DialogHeader>
          <DialogTitle className='text-xl font-semibold'>
            {t('Scan to Pay')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Open Alipay or WeChat on your phone and scan this QR code to complete payment. After paying, click “I have paid”.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='flex flex-col items-center gap-4 py-2'>
          {paymentMethod && (
            <div className='text-muted-foreground flex items-center gap-2 text-sm'>
              {getPaymentIcon(
                paymentMethod.type,
                'h-4 w-4',
                paymentMethod.icon,
                paymentMethod.name
              )}
              <span>{paymentMethod.name}</span>
            </div>
          )}
          <div className='rounded-xl border bg-white p-4 shadow-sm'>
            <QRCodeSVG value={qrContent} size={200} level='M' />
          </div>
        </div>

        <DialogFooter className='grid grid-cols-2 gap-2 sm:flex'>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Close')}
          </Button>
          <Button
            onClick={() => {
              onPaid?.()
              onOpenChange(false)
            }}
          >
            {t('I have paid')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
