/*
Copyright (C) 2023-2026 elesos

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
import {
  PAYMENT_TYPES,
  DEFAULT_PRESET_MULTIPLIERS,
  DEFAULT_PAYMENT_TYPE,
  DEFAULT_MIN_TOPUP,
} from '../constants'
import type { PresetAmount, TopupInfo } from '../types'

// ============================================================================
// Payment Processing Functions
// ============================================================================

/** How the frontend should launch an EasyPay (易支付) payment. */
export type EpayLaunchType = 'form' | 'url' | 'qrcode' | 'urlscheme'

export type EpayLaunchAction =
  | { type: 'form'; url: string; params: Record<string, unknown> }
  | { type: 'url'; url: string }
  | { type: 'qrcode'; content: string }
  | { type: 'urlscheme'; url: string; qrContent: string }

/**
 * Check if browser is Safari
 */
function isSafariBrowser(): boolean {
  return (
    navigator.userAgent.indexOf('Safari') > -1 &&
    navigator.userAgent.indexOf('Chrome') < 1
  )
}

/**
 * Detect mobile browsers. Used to decide whether urlscheme deep-links can open
 * the native Alipay/WeChat app directly.
 */
export function isMobileBrowser(): boolean {
  if (typeof navigator === 'undefined') {
    return false
  }
  return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(
    navigator.userAgent
  )
}

/**
 * Detect payment content that must be shown as a QR code on desktop instead of
 * being opened in a browser tab (Alipay qr.alipay.com / render.alipay.com,
 * WeChat weixin:// etc.).
 */
export function looksLikePaymentQRContent(raw: string): boolean {
  const s = (raw || '').trim().toLowerCase()
  if (!s) {
    return false
  }
  if (
    s.startsWith('weixin://') ||
    s.startsWith('wxp://') ||
    s.startsWith('alipays://') ||
    s.startsWith('alipay://')
  ) {
    return true
  }
  return (
    s.includes('qr.alipay.com') ||
    s.includes('render.alipay.com') ||
    s.includes('qr.weixin.qq.com') ||
    s.includes('wx.tenpay.com')
  )
}

function isURLScheme(raw: string): boolean {
  const s = (raw || '').trim().toLowerCase()
  return (
    s.startsWith('alipays://') ||
    s.startsWith('alipay://') ||
    s.startsWith('weixin://') ||
    s.startsWith('wxp://')
  )
}

/**
 * Build scannable QR content for a mobile payment scheme.
 * Alipay schemes are wrapped with the official render intermediate page so
 * phone cameras / Alipay scanners can open them reliably.
 */
export function buildPaymentQRContent(raw: string): string {
  const value = (raw || '').trim()
  if (!value) {
    return value
  }
  const lower = value.toLowerCase()
  if (lower.startsWith('alipays://') || lower.startsWith('alipay://')) {
    return `https://render.alipay.com/p/s/i?scheme=${encodeURIComponent(value)}`
  }
  return value
}

/**
 * Resolve how to launch an EasyPay payment from API response fields.
 *
 * Backend pay_type is preferred; client-side heuristics cover older backends
 * and gateways that put Alipay scan links into payurl.
 */
export function resolveEpayLaunch(
  url: string,
  params: Record<string, unknown> | undefined,
  payType?: string
): EpayLaunchAction {
  const safeParams = params || {}
  const entries = Object.entries(safeParams).filter(
    ([, value]) => value !== undefined && value !== null && value !== ''
  )
  const normalizedType = (payType || '').toLowerCase().trim()

  if (entries.length > 0 || normalizedType === 'form') {
    return { type: 'form', url, params: safeParams }
  }

  if (
    normalizedType === 'qrcode' ||
    (!normalizedType && looksLikePaymentQRContent(url))
  ) {
    return { type: 'qrcode', content: buildPaymentQRContent(url) }
  }

  if (normalizedType === 'urlscheme' || isURLScheme(url)) {
    return {
      type: 'urlscheme',
      url,
      qrContent: buildPaymentQRContent(url),
    }
  }

  // Heuristic: even when backend labels it "url", Alipay QR hosts must not open
  // in a desktop tab (they redirect to a broken app-wake page).
  if (looksLikePaymentQRContent(url)) {
    return { type: 'qrcode', content: buildPaymentQRContent(url) }
  }

  return { type: 'url', url }
}

/**
 * Open a browser payment URL (same-tab on Safari, new tab otherwise).
 */
export function openPaymentURL(url: string): void {
  if (isSafariBrowser()) {
    window.location.href = url
  } else {
    window.open(url, '_blank')
  }
}

/**
 * Submit a browser form POST for EasyPay page-jump mode.
 * Only use when resolveEpayLaunch returned type === 'form'.
 */
export function postPaymentForm(
  url: string,
  params: Record<string, unknown>
): void {
  const form = document.createElement('form')
  form.action = url
  form.method = 'POST'

  // Don't open in new tab for Safari
  if (!isSafariBrowser()) {
    form.target = '_blank'
  }

  Object.entries(params || {})
    .filter(
      ([, value]) => value !== undefined && value !== null && value !== ''
    )
    .forEach(([key, value]) => {
      const input = document.createElement('input')
      input.type = 'hidden'
      input.name = key
      input.value = String(value)
      form.appendChild(input)
    })

  document.body.appendChild(form)
  form.submit()
  document.body.removeChild(form)
}

/**
 * Submit payment form (for non-Stripe payments).
 *
 * When params are empty (e.g. EasyPay mapi.php returned a payurl), open the URL
 * directly instead of posting an empty form.
 *
 * Prefer resolveEpayLaunch for mapi qrcode/urlscheme flows — those must show a
 * QR dialog rather than opening Alipay intermediate pages in a new tab.
 *
 * Returns the resolved launch action so callers can display a QR modal when
 * needed. Legacy callers that ignore the return value still get form/url
 * launches; qrcode/urlscheme(desktop) are no-ops without a QR handler.
 */
export function submitPaymentForm(
  url: string,
  params: Record<string, unknown>,
  payType?: string
): EpayLaunchAction {
  const action = resolveEpayLaunch(url, params, payType)
  if (action.type === 'form') {
    postPaymentForm(action.url, action.params)
    return action
  }

  if (action.type === 'url') {
    openPaymentURL(action.url)
    return action
  }

  if (action.type === 'urlscheme' && isMobileBrowser()) {
    window.location.href = action.url
    return action
  }

  // qrcode / desktop urlscheme must be shown as a QR — do not open in a tab
  // (Alipay redirects to render.alipay.com and payment never completes).
  return action
}

/**
 * Check if payment method is Stripe
 */
export function isStripePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.STRIPE
}

/**
 * Check if payment method is Waffo Pancake
 *
 * Pancake is a metered-style payment that goes through a dedicated checkout
 * URL flow rather than the generic epay form submission, so it must be
 * special-cased in payment dispatch logic.
 */
export function isWaffoPancakePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.WAFFO_PANCAKE
}

/**
 * Get default payment type from topup info
 */
export function getDefaultPaymentType(topupInfo: TopupInfo | null): string {
  if (!topupInfo) {
    return DEFAULT_PAYMENT_TYPE
  }

  // Return first available payment method or default
  if (topupInfo.pay_methods?.length > 0) {
    return topupInfo.pay_methods[0].type
  }

  if (topupInfo.enable_stripe_topup) {
    return PAYMENT_TYPES.STRIPE
  }

  if (topupInfo.enable_waffo_topup) {
    return PAYMENT_TYPES.WAFFO
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return PAYMENT_TYPES.WAFFO_PANCAKE
  }

  return DEFAULT_PAYMENT_TYPE
}

/**
 * Get minimum topup amount from topup info
 */
export function getMinTopupAmount(topupInfo: TopupInfo | null): number {
  if (!topupInfo) {
    return DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_online_topup) {
    return topupInfo.min_topup
  }

  if (topupInfo.enable_stripe_topup) {
    return topupInfo.stripe_min_topup
  }

  if (topupInfo.enable_waffo_topup) {
    return topupInfo.waffo_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return topupInfo.waffo_pancake_min_topup || DEFAULT_MIN_TOPUP
  }

  return DEFAULT_MIN_TOPUP
}

/**
 * Generate preset amounts based on minimum topup
 */
export function generatePresetAmounts(minAmount: number): PresetAmount[] {
  return DEFAULT_PRESET_MULTIPLIERS.map((multiplier) => ({
    value: minAmount * multiplier,
  }))
}

/**
 * Merge custom preset amounts with discounts
 */
export function mergePresetAmounts(
  amountOptions: number[],
  discounts: Record<number, number>
): PresetAmount[] {
  if (!amountOptions || amountOptions.length === 0) {
    return []
  }

  return amountOptions.map((amount) => ({
    value: amount,
    discount: discounts[amount] || 1.0,
  }))
}
