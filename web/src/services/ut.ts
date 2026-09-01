import { api } from '../stores/auth'
import { callService, errorMessage } from './http'

export type UtToggle = {
  active: boolean
  target?: string
}

export type UtIdentityRestriction = {
  active: boolean
  restricted: boolean
}

export type UtSimservs = {
  xui: string
  etag: string
  communication_diversion: UtToggle
  identity_restriction: UtIdentityRestriction
  incoming_barring: UtToggle
  outgoing_barring: UtToggle
}

export const utService = {
  async getSimservs(deviceId: string) {
    return callService(async () => {
      const res = await api.get<UtSimservs>(`/devices/${encodeURIComponent(deviceId)}/ut/simservs`)
      return res.data
    })
  },
  async putSimservs(deviceId: string, body: Record<string, unknown>) {
    return callService(async () => {
      const res = await api.put<UtSimservs>(`/devices/${encodeURIComponent(deviceId)}/ut/simservs`, body)
      return res.data
    })
  },
  message(err: unknown) {
    return errorMessage(err, '呼叫设置请求失败')
  }
}
