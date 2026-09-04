import { reactive } from 'vue'
import { phoneContactsService, type PhoneIdentity } from '../services/phone-contacts'

const cache = reactive(new Map<string, PhoneIdentity>())
const inflight = new Map<string, Promise<PhoneIdentity>>()

function cacheKey(number: string) {
  return String(number || '').trim()
}

export function usePhoneIdentity() {
  function identityFor(number?: string): PhoneIdentity | undefined {
    const key = cacheKey(number || '')
    if (!key) return undefined
    return cache.get(key)
  }

  async function resolve(number?: string): Promise<PhoneIdentity | undefined> {
    const key = cacheKey(number || '')
    if (!key) return undefined
    const hit = cache.get(key)
    if (hit) return hit
    const pending = inflight.get(key)
    if (pending) return pending
    const req = phoneContactsService.lookup(key).then((ident) => {
      cache.set(key, ident)
      if (ident.number && ident.number !== key) cache.set(ident.number, ident)
      if (ident.display_number) cache.set(ident.display_number, ident)
      return ident
    }).finally(() => {
      inflight.delete(key)
    })
    inflight.set(key, req)
    return req
  }

  function remember(ident: PhoneIdentity) {
    if (!ident) return
    if (ident.number) cache.set(ident.number, ident)
    if (ident.display_number) cache.set(ident.display_number, ident)
  }

  function titleFor(number?: string) {
    return identityFor(number)?.title || number || '未知号码'
  }

  function subtitleFor(number?: string) {
    return identityFor(number)?.subtitle || ''
  }

  return { identityFor, resolve, remember, titleFor, subtitleFor }
}
