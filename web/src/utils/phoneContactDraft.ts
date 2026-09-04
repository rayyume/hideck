export const PHONE_CONTACT_NUMBER_PATTERN = /^\+?[0-9]{1,32}$/
export const PHONE_CONTACT_NUMBER_ERROR = '号码只能是数字，可带开头的 +'

export function normalizePhoneContactNumber(value: unknown) {
  return String(value || '').trim()
}

export function isPhoneContactNumberValid(value: unknown) {
  return PHONE_CONTACT_NUMBER_PATTERN.test(normalizePhoneContactNumber(value))
}

export function phoneContactNumberError(values: readonly string[]) {
  const hasInvalidNumber = values.some((value) => {
    const normalized = normalizePhoneContactNumber(value)
    return normalized !== '' && !PHONE_CONTACT_NUMBER_PATTERN.test(normalized)
  })
  return hasInvalidNumber ? PHONE_CONTACT_NUMBER_ERROR : ''
}

export function validPhoneContactNumbers(primary: string, extras: readonly string[]) {
  if (!isPhoneContactNumberValid(primary) || phoneContactNumberError([primary, ...extras])) return []
  const numbers = [primary, ...extras]
    .map(normalizePhoneContactNumber)
    .filter(Boolean)
  return Array.from(new Set(numbers))
}
