import { reactive } from 'vue'
import {
  phoneContactsService,
  type PhoneContactsPage,
  type PhoneIdentity
} from '../services/phone-contacts'

const CONTACT_PAGE_SIZE = 100

type ContactReloadOptions = Readonly<{ fresh?: boolean }>

const cache = reactive(new Map<string, PhoneIdentity>())
const inflight = new Map<string, Promise<PhoneIdentity>>()
const contacts = reactive<PhoneIdentity[]>([])
const contactsState = reactive({
  loaded: false,
  loading: false,
  loadingMore: false,
  error: '',
  total: 0,
  nextOffset: 0,
  hasMore: false
})
let contactsLoadPromise: Promise<PhoneIdentity[]> | undefined
let contactsMorePromise: Promise<PhoneIdentity[]> | undefined
let contactsGeneration = 0
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
  delete result.contact_id
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
  else {
    contacts.unshift(ident)
    contactsState.total += 1
    contactsState.nextOffset += 1
  }
}

function rememberWithoutList(ident: PhoneIdentity, sourceNumber = '', deviceId = '') {
  if (!ident) return
  if (ident.number) cache.set(ident.number, ident)
  const source = cacheKey(sourceNumber, deviceId)
  if (source) cache.set(source, ident)
  const display = cacheKey(ident.display_number, deviceId)
  if (display) cache.set(display, ident)
}

function reloadContacts(options: ContactReloadOptions = {}) {
  if (contactsLoadPromise && !options.fresh) return contactsLoadPromise
  const generation = ++contactsGeneration
  contactsState.loading = true
  contactsState.error = ''
  let request: Promise<PhoneIdentity[]>
  request = loadCurrentContactPage(0, generation).then((page) => {
    if (generation !== contactsGeneration) return contacts
    replaceContactRows(page)
    return contacts
  }).catch((error: unknown) => {
    if (generation === contactsGeneration) contactsState.error = errorMessage(error)
    throw error
  }).finally(() => {
    if (contactsLoadPromise === request) {
      contactsState.loading = false
      contactsLoadPromise = undefined
    }
  })
  contactsLoadPromise = request
  return request
}

async function loadCurrentContactPage(offset: number, generation: number) {
  for (;;) {
    const revision = cacheRevision
    const page = await phoneContactsService.listPage({ limit: CONTACT_PAGE_SIZE, offset })
    if (generation !== contactsGeneration || revision === cacheRevision) return page
  }
}

function replaceContactRows(page: PhoneContactsPage) {
  cacheRevision++
  contacts.splice(0, contacts.length, ...page.contacts)
  for (const row of page.contacts) rememberWithoutList(row)
  if (!page.hasMore) reconcileContactNameCache(page.contacts)
  contactsState.total = page.total
  contactsState.nextOffset = page.nextOffset
  contactsState.hasMore = page.hasMore
  contactsState.loaded = true
}

function appendContactRows(page: PhoneContactsPage) {
  const loaded = new Set(contacts.map((row) => row.number))
  const rows = page.contacts.filter((row) => !loaded.has(row.number))
  contacts.push(...rows)
  for (const row of rows) rememberWithoutList(row)
  if (!page.hasMore) {
    cacheRevision++
    reconcileContactNameCache(contacts)
  }
  contactsState.total = page.total
  contactsState.nextOffset = page.nextOffset
  contactsState.hasMore = page.hasMore
}

function reconcileContactNameCache(rows: readonly PhoneIdentity[]) {
  const currentNumbers = new Set(rows.map((row) => row.number))
  for (const [key, value] of cache) {
    if (value.name && !currentNumbers.has(value.number)) cache.set(key, withoutContactName(value))
  }
}

function loadMoreContacts() {
  if (!contactsState.loaded) return ensureContacts()
  if (contactsState.loading) return contactsLoadPromise || Promise.resolve(contacts)
  if (!contactsState.hasMore) return Promise.resolve(contacts)
  if (contactsMorePromise) return contactsMorePromise
  const generation = contactsGeneration
  contactsState.loadingMore = true
  contactsState.error = ''
  let request: Promise<PhoneIdentity[]>
  request = loadCurrentContactPage(contactsState.nextOffset, generation).then((page) => {
    if (generation === contactsGeneration) appendContactRows(page)
    return contacts
  }).catch((error: unknown) => {
    if (generation === contactsGeneration) contactsState.error = errorMessage(error)
    throw error
  }).finally(() => {
    if (contactsMorePromise === request) {
      contactsState.loadingMore = false
      contactsMorePromise = undefined
    }
  })
  contactsMorePromise = request
  return request
}

async function loadAllContacts() {
  await ensureContacts()
  while (contactsState.hasMore) await loadMoreContacts()
  return contacts
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
  const deleted = await phoneContactsService.removeMany(targets, deviceId)
  removeLocalContacts(targets, deleted)
}

function applyContactGroupUpdate(rows: readonly PhoneIdentity[]) {
  if (!rows.length) return
  cacheRevision++
  const updated = new Map(rows.map((row) => [row.number, row]))
  for (const [alias, value] of cache) {
    const replacement = updated.get(value.number)
    if (replacement) cache.set(alias, replacement)
  }
  for (const row of rows) rememberWithoutList(row)
  const loadedCount = contacts.length
  const reordered = [...rows, ...contacts.filter((item) => !updated.has(item.number))]
  contacts.splice(0, contacts.length, ...reordered.slice(0, loadedCount))
  contactsState.nextOffset = contacts.length
  contactsState.hasMore = contactsState.nextOffset < contactsState.total
}

async function removeContactGroup(contactId: string) {
  const result = await phoneContactsService.removeGroup(contactId)
  removeLocalContacts(result.numbers, result.deleted)
}

function removeLocalContacts(numbers: readonly string[], deleted: number) {
  contactsGeneration++
  cacheRevision++
  const drop = new Set(numbers)
  const loadedDeleted = contacts.filter((item) => drop.has(item.number)).length
  for (let i = contacts.length - 1; i >= 0; i--) {
    if (drop.has(contacts[i].number)) contacts.splice(i, 1)
  }
  contactsState.total = Math.max(0, contactsState.total - deleted)
  contactsState.nextOffset = Math.min(
    contactsState.total,
    Math.max(0, contactsState.nextOffset - loadedDeleted)
  )
  contactsState.hasMore = contactsState.nextOffset < contactsState.total
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
  get contactsLoadingMore() { return contactsState.loadingMore },
  get contactsError() { return contactsState.error },
  get contactsTotal() { return contactsState.total },
  get contactsHasMore() { return contactsState.hasMore },
  identityFor,
  resolve,
  remember,
  upsertLocal,
  reloadContacts,
  loadMoreContacts,
  loadAllContacts,
  ensureContacts,
  removeContact,
  removeContacts,
  applyContactGroupUpdate,
  removeContactGroup,
  titleFor,
  subtitleFor
}

export function usePhoneIdentity() {
  return phoneIdentity
}
