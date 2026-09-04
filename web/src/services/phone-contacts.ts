import { api } from '../stores/auth'

export type PhoneIdentity = {
  number: string
  display_number: string
  contact_id?: string
  name?: string
  title: string
  subtitle: string
  carrier?: string
  region?: string
  country?: string
  kind: string
}

export type PhoneContactsPage = Readonly<{
  contacts: PhoneIdentity[]
  total: number
  nextOffset: number
  hasMore: boolean
}>

type PhoneContactsPageParams = Readonly<{
  limit: number
  offset: number
}>

export type PhoneContactSave = Readonly<{
  number: string
  name: string
  deviceId?: string
  contactId?: string
  groupKey?: string
}>

function normalizeIdentity(row: PhoneIdentity): PhoneIdentity {
  return {
    number: row.number,
    display_number: row.display_number || row.number,
    contact_id: row.contact_id,
    name: row.name,
    title: row.title || row.name || row.display_number || row.number,
    subtitle: row.subtitle || '',
    carrier: row.carrier,
    region: row.region,
    country: row.country,
    kind: row.kind || 'contact'
  }
}

function pageInteger(value: unknown, field: string): number {
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error(`联系人分页响应 ${field} 无效`)
  }
  return parsed
}

export const phoneContactsService = {
  async lookup(number: string, deviceId = ''): Promise<PhoneIdentity> {
    const res = await api.get('/phone/lookup', {
      params: { number, device_id: deviceId || undefined }
    })
    return res.data as PhoneIdentity
  },
  async list(): Promise<PhoneIdentity[]> {
    const res = await api.get('/phone/contacts')
    return ((res.data?.contacts || []) as PhoneIdentity[]).map(normalizeIdentity)
  },
  async listPage(params: PhoneContactsPageParams): Promise<PhoneContactsPage> {
    const res = await api.get('/phone/contacts', { params })
    if (!Array.isArray(res.data?.contacts)) {
      throw new Error('联系人分页响应 contacts 无效')
    }
    const contacts = (res.data.contacts as PhoneIdentity[]).map(normalizeIdentity)
    if (typeof res.data?.has_more !== 'boolean') {
      throw new Error('联系人分页响应 has_more 无效')
    }
    const nextOffset = pageInteger(res.data?.next_offset, 'next_offset')
    if (res.data.has_more && nextOffset <= params.offset) {
      throw new Error('联系人分页响应 next_offset 没有前进')
    }
    return {
      contacts,
      total: pageInteger(res.data?.total, 'total'),
      nextOffset,
      hasMore: res.data.has_more
    }
  },
  async save(contact: PhoneContactSave): Promise<PhoneIdentity> {
    const res = await api.put('/phone/contacts', {
      number: contact.number,
      name: contact.name,
      device_id: contact.deviceId || undefined,
      contact_id: contact.contactId || undefined
    })
    return normalizeIdentity(res.data as PhoneIdentity)
  },
  async saveMany(contacts: readonly PhoneContactSave[], deviceId = ''): Promise<PhoneIdentity[]> {
    const res = await api.post('/phone/contacts/batch', {
      device_id: deviceId || undefined,
      atomic: true,
      contacts: contacts.map((contact) => ({
        number: contact.number,
        name: contact.name,
        contact_id: contact.contactId || undefined,
        group_key: contact.groupKey || undefined
      }))
    })
    if (!Array.isArray(res.data?.contacts) || res.data.contacts.length !== contacts.length) {
      throw new Error('批量保存联系人响应不完整')
    }
    return (res.data.contacts as PhoneIdentity[]).map(normalizeIdentity)
  },
  async updateGroup(contactId: string, name: string): Promise<PhoneIdentity[]> {
    const res = await api.put('/phone/contacts/group', {
      contact_id: contactId,
      name
    })
    const contacts = res.data?.contacts
    if (!Array.isArray(contacts) || contacts.length === 0 || contacts.some((contact) =>
      typeof contact?.number !== 'string' || !contact.number || contact.contact_id !== contactId)) {
      throw new Error('更新联系人响应不完整')
    }
    return (contacts as PhoneIdentity[]).map(normalizeIdentity)
  },
  async removeGroup(contactId: string): Promise<{ deleted: number; numbers: string[] }> {
    const res = await api.delete('/phone/contacts/group', {
      params: { contact_id: contactId }
    })
    const deleted = Number(res.data?.deleted)
    const numbers = res.data?.numbers
    if (!Number.isInteger(deleted) || deleted < 1 || !Array.isArray(numbers) ||
      numbers.length !== deleted || numbers.some((number) => typeof number !== 'string' || !number.trim())) {
      throw new Error('删除联系人响应不完整')
    }
    return { deleted, numbers: [...numbers] }
  },
  async remove(number: string, deviceId = ''): Promise<void> {
    await api.delete('/phone/contacts', {
      params: { number, device_id: deviceId || undefined }
    })
  },
  async removeMany(numbers: string[], deviceId = ''): Promise<number> {
    const res = await api.post('/phone/contacts/delete', {
      numbers,
      device_id: deviceId || undefined
    })
    return Number(res.data?.deleted || 0)
  },
  async importFile(file: File, deviceId = ''): Promise<{ imported: number; skipped: number; parsed: number }> {
    const body = new FormData()
    body.append('file', file)
    if (deviceId) body.append('device_id', deviceId)
    const res = await api.post('/phone/contacts/import', body)
    return {
      imported: Number(res.data?.imported || 0),
      skipped: Number(res.data?.skipped || 0),
      parsed: Number(res.data?.parsed || 0)
    }
  },
  async exportFile(format: 'vcf' | 'csv' = 'vcf'): Promise<void> {
    const res = await api.get('/phone/contacts/export', {
      params: { format },
      responseType: 'blob'
    })
    const blob = new Blob([res.data], {
      type: format === 'csv' ? 'text/csv;charset=utf-8' : 'text/vcard;charset=utf-8'
    })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    const stamp = new Date().toISOString().slice(0, 10).replace(/-/g, '')
    link.href = url
    link.download = `hideck-contacts-${stamp}.${format}`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  }
}
