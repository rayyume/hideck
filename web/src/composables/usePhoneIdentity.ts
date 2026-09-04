import { reactive } from 'vue'
import { phoneContactsService, type PhoneIdentity } from '../services/phone-contacts'

const cache = reactive(new Map<string, PhoneIdentity>())
const inflight = new Map<string, Promise<PhoneIdentity>>()
const contacts = reactive<PhoneIdentity[]>([])
const contactsState = reactive({ loaded: false, loading: false, error: '' })
let contactsLoadPromise: Promise<PhoneIdentity[]> | undefined
let cacheRevision = 0

function normalizedKey(value?: string) {
  return String(value || '').trim()
}

function cacheKey(number?: string, deviceId?: string) {
  const normalized = normalizedKey(number)
  const device = normalizedKey(deviceId)
  return device ? `${device}\u0000${normalized}` : normalized
}

function withoutContactName(ident: PhoneIdentity): PhoneIdentity {
  const result = { ...ident }
  delete result.name
  result.title = ident.display_number || ident.number
  return result
}

function errorMessage(error: unknown) {
  const responseMessage = (error as { response?: { data?: { message?: unknown } } })
    ?.response?.data?.message
  if (typeof responseMessage === 'string' && responseMessage.trim()) return responseMessage.trim()
  return error instanceof Error && error.message ? error.message : '加载联系人失败'
}

function identityFor(number?: string, deviceId?: string): PhoneIdentity | undefined {
  const key = cacheKey(number, deviceId)
  if (!key) return undefined
  return cache.get(key) || cache.get(normalizedKey(number))
}

async function resolve(number?: string, deviceId?: string): Promise<PhoneIdentity | undefined> {
  const raw = normalizedKey(number)
  const key = cacheKey(raw, deviceId)
  if (!key) return undefined
  const hit = identityFor(raw, deviceId)
  if (hit) return hit
  const pending = inflight.get(key)
  if (pending) return pending
  const revision = cacheRevision
  const request = phoneContactsService.lookup(raw, normalizedKey(deviceId)).then((ident) => {
    if (revision === cacheRevision) rememberWithoutList(ident, raw, deviceId)
    return ident
  }).finally(() => {
    inflight.delete(key)
  })
  inflight.set(key, request)
  return request
}

function remember(ident: PhoneIdentity, sourceNumber = '', deviceId = '') {
  rememberWithoutList(ident, sourceNumber, deviceId)
}

function upsertLocal(ident: PhoneIdentity, sourceNumber = '', deviceId = '') {
  if (!ident?.number) return
  cacheRevision++
  for (const [key, value] of cache) {
    if (value.number === ident.number) cache.set(key, ident)
  }
  rememberWithoutList(ident, sourceNumber, deviceId)
  const index = contacts.findIndex((item) => item.number === ident.number)
  if (index >= 0) contacts.splice(index, 1, ident)
  else contacts.unshift(ident)
}

function rememberWithoutList(ident: PhoneIdentity, sourceNumber = '', deviceId = '') {
  if (!ident) return
  if (ident.number) cache.set(ident.number, ident)
  const source = cacheKey(sourceNumber, deviceId)
  if (source) cache.set(source, ident)
  const display = cacheKey(ident.display_number, deviceId)
  if (display) cache.set(display, ident)
}

function reloadContacts() {
  if (contactsLoadPromise) return contactsLoadPromise
  contactsState.loading = true
  contactsState.error = ''
  const request = loadCurrentContactRows().then((rows) => {
    applyContactRows(rows)
    return rows
  }).catch((error: unknown) => {
    contactsState.error = errorMessage(error)
    throw error
  }).finally(() => {
    contactsState.loading = false
    contactsLoadPromise = undefined
  })
  contactsLoadPromise = request
  return request
}

async function loadCurrentContactRows() {
  for (;;) {
    const revision = cacheRevision
    const rows = await phoneContactsService.list()
    if (revision === cacheRevision) return rows
  }
}

function applyContactRows(rows: PhoneIdentity[]) {
  const currentNumbers = new Set(rows.map((row) => row.number))
  cacheRevision++
  for (const [key, value] of cache) {
    if (value.name && !currentNumbers.has(value.number)) cache.set(key, withoutContactName(value))
  }
  contacts.splice(0, contacts.length, ...rows)
  for (const row of rows) rememberWithoutList(row)
  contactsState.loaded = true
}

async function ensureContacts() {
  if (!contactsState.loaded) await reloadContacts()
  return contacts
}

async function removeContact(number: string, deviceId = '') {
  await removeContacts([number], deviceId)
}

async function removeContacts(numbers: string[], deviceId = '') {
  const targets = numbers.map((number) => {
    const key = normalizedKey(number)
    return contacts.find((item) => item.number === key || item.display_number === key)?.number || key
  }).filter(Boolean)
  if (!targets.length) return
  await phoneContactsService.removeMany(targets, deviceId)
  cacheRevision++
  const drop = new Set(targets)
  for (let i = contacts.length - 1; i >= 0; i--) {
    if (drop.has(contacts[i].number)) contacts.splice(i, 1)
  }
  for (const [alias, value] of cache) {
    if (drop.has(value.number)) cache.set(alias, withoutContactName(value))
  }
}

function titleFor(number?: string, deviceId?: string) {
  return identityFor(number, deviceId)?.title || number || '未知号码'
}

function subtitleFor(number?: string, deviceId?: string) {
  return identityFor(number, deviceId)?.subtitle || ''
}

const phoneIdentity = {
  contacts,
  get contactsLoaded() { return contactsState.loaded },
  get contactsLoading() { return contactsState.loading },
  get contactsError() { return contactsState.error },
  identityFor,
  resolve,
  remember,
  upsertLocal,
  reloadContacts,
  ensureContacts,
  removeContact,
  removeContacts,
  titleFor,
  subtitleFor
}

export function usePhoneIdentity() {
  return phoneIdentity
}
