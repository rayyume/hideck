import { reactive } from 'vue'
import { phoneContactsService, type PhoneIdentity } from '../services/phone-contacts'

const cache = reactive(new Map<string, PhoneIdentity>())
const inflight = new Map<string, Promise<PhoneIdentity>>()
const contacts = reactive<PhoneIdentity[]>([])
let contactsLoaded = false

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
    rememberWithoutList(ident)
  }

  function upsertLocal(ident: PhoneIdentity) {
    if (!ident?.number) return
    rememberWithoutList(ident)
    const index = contacts.findIndex((item) => item.number === ident.number)
    if (index >= 0) contacts.splice(index, 1, ident)
    else contacts.unshift(ident)
  }

  function rememberWithoutList(ident: PhoneIdentity) {
    if (!ident) return
    if (ident.number) cache.set(ident.number, ident)
    if (ident.display_number) cache.set(ident.display_number, ident)
  }

  async function reloadContacts() {
    const rows = await phoneContactsService.list()
    contacts.splice(0, contacts.length, ...rows)
    for (const row of rows) rememberWithoutList(row)
    contactsLoaded = true
    return contacts
  }

  async function ensureContacts() {
    if (!contactsLoaded) await reloadContacts()
    return contacts
  }

  async function removeContact(number: string) {
    const key = cacheKey(number)
    await phoneContactsService.remove(key)
    const index = contacts.findIndex((item) => item.number === key || item.display_number === key)
    if (index >= 0) contacts.splice(index, 1)
  }

  function titleFor(number?: string) {
    return identityFor(number)?.title || number || '未知号码'
  }

  function subtitleFor(number?: string) {
    return identityFor(number)?.subtitle || ''
  }

  return {
    contacts,
    identityFor,
    resolve,
    remember,
    upsertLocal,
    reloadContacts,
    ensureContacts,
    removeContact,
    titleFor,
    subtitleFor
  }
}
