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

export const phoneContactsService = {
  async lookup(number: string, deviceId = ''): Promise<PhoneIdentity> {
    const res = await api.get('/phone/lookup', {
      params: { number, device_id: deviceId || undefined }
    })
    return res.data as PhoneIdentity
  },
  async list(): Promise<PhoneIdentity[]> {
    const res = await api.get('/phone/contacts')
    return ((res.data?.contacts || []) as PhoneIdentity[]).map((row) => ({
      number: row.number,
      display_number: row.display_number || row.number,
      name: row.name,
      title: row.title || row.name || row.display_number || row.number,
      subtitle: row.subtitle || '',
      carrier: row.carrier,
      region: row.region,
      country: row.country,
      kind: row.kind || 'contact'
    }))
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
