import { api } from '../stores/auth'

export type PhoneIdentity = {
  number: string
  display_number: string
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

function normalizeIdentity(row: PhoneIdentity): PhoneIdentity {
  return {
    number: row.number,
    display_number: row.display_number || row.number,
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
  async save(number: string, name: string, deviceId = ''): Promise<PhoneIdentity> {
    const res = await api.put('/phone/contacts', {
      number,
      name,
      device_id: deviceId || undefined
    })
    return res.data as PhoneIdentity
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
