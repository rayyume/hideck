import { ref, watch, type Ref } from 'vue'
import type { CardPolicy } from '../types/api'

type EditablePolicyFields = Pick<CardPolicy, 'ip_version' | 'apn' | 'vowifi_upstream_proxy_id'>
type SaveResult = { ok: boolean; error?: { message?: string } }

export function useCardPolicyFields(
  source: Ref<CardPolicy | null>,
  save: (patch: Partial<EditablePolicyFields>) => Promise<SaveResult>,
  onChanged?: () => void
) {
  const ipVersion = ref<CardPolicy['ip_version']>('v4')
  const apn = ref('')
  const vowifiUpstreamProxyID = ref('')
  const pending = ref<keyof EditablePolicyFields | null>(null)
  const error = ref('')
  const errorField = ref<keyof EditablePolicyFields | null>(null)

  watch(source, (policy) => {
    if (!policy || pending.value) return
    ipVersion.value = policy.ip_version || 'v4'
    apn.value = policy.apn || ''
    vowifiUpstreamProxyID.value = policy.vowifi_upstream_proxy_id || ''
  }, { immediate: true })

  function currentValue(field: keyof EditablePolicyFields) {
    if (field === 'ip_version') return ipVersion.value
    if (field === 'apn') return apn.value
    return vowifiUpstreamProxyID.value
  }

  function restore(field: keyof EditablePolicyFields) {
    if (!source.value) return
    if (field === 'ip_version') ipVersion.value = source.value.ip_version
    else if (field === 'apn') apn.value = source.value.apn || ''
    else vowifiUpstreamProxyID.value = source.value.vowifi_upstream_proxy_id || ''
  }

  async function persist(field: keyof EditablePolicyFields) {
    if (!source.value || pending.value) return
    const previous = source.value[field] || ''
    if (field === 'apn') apn.value = apn.value.trim()
    const value = currentValue(field)
    if (value === previous) return

    pending.value = field
    error.value = ''
    errorField.value = null
    const result = await save({ [field]: value })
    pending.value = null
    if (!result.ok) {
      restore(field)
      error.value = result.error?.message || '策略保存失败'
      errorField.value = field
      return
    }
    onChanged?.()
  }

  return {
    ipVersion, apn, vowifiUpstreamProxyID, pending, error, errorField,
    saveIPVersion: () => persist('ip_version'),
    saveAPN: () => persist('apn'),
    saveVowifiUpstreamProxy: () => persist('vowifi_upstream_proxy_id')
  }
}
