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
  async lookup(number: string): Promise<PhoneIdentity> {
    const res = await api.get('/phone/lookup', { params: { number } })
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
  async save(number: string, name: string): Promise<PhoneIdentity> {
    const res = await api.put('/phone/contacts', { number, name })
    return res.data as PhoneIdentity
  },
  async remove(number: string): Promise<void> {
    await api.delete('/phone/contacts', { params: { number } })
  }
}
