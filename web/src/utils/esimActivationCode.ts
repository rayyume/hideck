export type ParsedEsimActivation = {
  smdp: string
  matchingId: string
  oid: string
  confirmationRequired: boolean
  confirmationCode: string
}

export function looksLikeEsimActivationCode(raw: string) {
  const value = raw.trim()
  if (!value) return false
  const upper = value.toUpperCase()
  if (upper.includes('LPA:') || /^1\$/.test(value) || /(?:\?|&)carddata=/i.test(value)) return true
  if (parseLabeledActivation(value) || parseTwoLineActivation(value)) return true
  return /^[A-Za-z0-9.-]+\.[A-Za-z0-9.-]+\$/.test(value.replace(/\s+/g, ''))
}

export function parseEsimActivationInput(raw: string): ParsedEsimActivation | null {
  return parseCompactActivation(raw) || parseLabeledActivation(raw) || parseTwoLineActivation(raw) || parseHostOnly(raw)
}

type TransferLike = {
  files?: ArrayLike<File> | null
  items?: ArrayLike<{ kind?: string; type?: string; getAsFile?: () => File | null }> | null
  types?: ArrayLike<string> | null
}

export function transferLooksLikeImageDrop(data: TransferLike | null) {
  if (!data) return false
  if (pickClipboardOrDropImage(data)) return true
  const types = data.types ? Array.from(data.types) : []
  return types.some((type) => type === 'Files' || type.startsWith('image/'))
}

export function pickClipboardOrDropImage(data: TransferLike | null): File | null {
  if (!data) return null
  const files = data.files ? Array.from(data.files) : []
  const fromFiles = files.find(isImageFile)
  if (fromFiles) return fromFiles
  const items = data.items ? Array.from(data.items) : []
  for (const item of items) {
    if (item.kind === 'file' && isImageType(item.type || '')) {
      const file = item.getAsFile?.()
      if (file) return file
    }
  }
  return null
}

export async function readQrPayloadFromImageFile(image: Blob): Promise<string> {
  const Detector = (globalThis as typeof globalThis & {
    BarcodeDetector?: new (options?: { formats?: string[] }) => {
      detect(source: ImageBitmapSource): Promise<Array<{ rawValue?: string }>>
    }
  }).BarcodeDetector
  if (!Detector) {
    throw new Error('当前浏览器不能直接识别二维码图片，请把二维码扫出来，把 LPA:1$ 开头的激活码贴进来')
  }
  const detector = new Detector({ formats: ['qr_code'] })
  const bitmap = await createImageBitmap(image)
  try {
    const codes = await detector.detect(bitmap)
    const value = codes.map((code) => (code.rawValue || '').trim()).find(Boolean)
    if (!value) {
      throw new Error('图片里没有识别到二维码，请换一张更清晰的截图')
    }
    return value
  } finally {
    bitmap.close()
  }
}

function isImageType(type: string) {
  return type.startsWith('image/')
}

function isImageFile(file: File) {
  return isImageType(file.type) || /\.(png|jpe?g|webp|gif|bmp|heic|heif)$/i.test(file.name)
}

function emptyActivation(): ParsedEsimActivation {
  return { smdp: '', matchingId: '', oid: '', confirmationRequired: false, confirmationCode: '' }
}

function parseLabeledActivation(raw: string): ParsedEsimActivation | null {
  const smdp = matchField(raw, [
    /SM-?DP\+?\s*(?:Address|地址)?\s*[:：]\s*(\S+)/i,
    /(?:服务器地址|下载地址)\s*[:：]\s*(\S+)/i
  ])
  if (!smdp) return null
  const host = stripScheme(smdp)
  if (!looksLikeHost(host)) return null
  const matchingId = matchField(raw, [
    /Matching\s*ID\s*[:：]\s*(\S+)/i,
    /Activation\s*Code\s*[:：]\s*(\S+)/i,
    /AC(?:tivation)?\s*Token\s*[:：]\s*(\S+)/i,
    /(?:激活码|匹配码)\s*[:：]\s*(\S+)/
  ])
  if (!looksLikeMatchingId(matchingId)) return null
  const confirmationCode = matchField(raw, [
    /Confirmation\s*Code\s*[:：]\s*(\S+)/i,
    /确认码\s*[:：]\s*(\S+)/
  ]) || ''
  return {
    ...emptyActivation(),
    smdp: host,
    matchingId,
    confirmationRequired: Boolean(confirmationCode),
    confirmationCode
  }
}

function parseTwoLineActivation(raw: string): ParsedEsimActivation | null {
  const lines = raw.split(/\r?\n/).map((line) => line.trim()).filter(Boolean)
  if (lines.length !== 2) return null
  const [first, second] = lines
  if (looksLikeHost(first) && looksLikeMatchingId(second)) {
    return { ...emptyActivation(), smdp: stripScheme(first), matchingId: second }
  }
  if (looksLikeHost(second) && looksLikeMatchingId(first)) {
    return { ...emptyActivation(), smdp: stripScheme(second), matchingId: first }
  }
  return null
}

function parseCompactActivation(raw: string): ParsedEsimActivation | null {
  const extracted = extractActivationCode(raw)
  if (!extracted) return null
  return parseLpaBody(extracted)
}

function extractActivationCode(raw: string): string | null {
  let value = raw.replace(/\s+/g, '').trim()
  if (!value) return null

  if (/%[0-9A-Fa-f]{2}/.test(value)) {
    try {
      value = decodeURIComponent(value)
    } catch {
      // keep the original text when it is not valid percent-encoding
    }
  }

  const fromQuery = activationFromQuery(value)
  if (fromQuery) value = fromQuery.replace(/\s+/g, '').trim()

  const lpaIndex = value.toUpperCase().indexOf('LPA:')
  if (lpaIndex >= 0) {
    return normalizeLpaPrefix(value.slice(lpaIndex))
  }
  if (/^1\$/.test(value)) {
    return `LPA:${value}`
  }
  const hostToken = value.match(/^((?:https?:\/\/)?[A-Za-z0-9.-]+\.[A-Za-z0-9.-]+(?::\d+)?)\$([^$]+)(?:\$([^$]*))?(?:\$([^$]*))?$/i)
  if (hostToken) {
    return ['LPA:1', stripScheme(hostToken[1]), hostToken[2], hostToken[3] || '', hostToken[4] || ''].join('$')
  }
  return null
}

function parseHostOnly(raw: string): ParsedEsimActivation | null {
  const lines = raw.split(/\r?\n/).map((line) => line.trim()).filter(Boolean)
  if (lines.length !== 1) return null
  const host = stripScheme(lines[0])
  if (!looksLikeHost(host)) return null
  return { ...emptyActivation(), smdp: host }
}

function activationFromQuery(value: string): string | null {
  try {
    const parsed = new URL(value)
    for (const key of ['carddata', 'activationcode', 'activation_code', 'lpa', 'data']) {
      const found = parsed.searchParams.get(key)?.trim()
      if (found) return found
    }
  } catch {
    return null
  }
  return null
}

function normalizeLpaPrefix(value: string) {
  if (/^lpa:\/\//i.test(value)) {
    return `LPA:${value.slice(6)}`
  }
  if (/^lpa:/i.test(value)) {
    return `LPA:${value.slice(4)}`
  }
  return value
}

function parseLpaBody(code: string): ParsedEsimActivation | null {
  const normalized = normalizeLpaPrefix(code)
  if (!/^LPA:1/i.test(normalized)) return null
  const parts = normalized.split('$')
  const smdp = stripScheme(parts[1] || '')
  if (!smdp) return null

  const matchingId = (parts[2] || '').trim()
  let oid = ''
  let confirmationRequired = false
  if (parts.length >= 5) {
    oid = parts[3] || ''
    confirmationRequired = parts[4] === '1'
  } else if (parts.length === 4) {
    if (parts[3] === '1') {
      confirmationRequired = true
    } else {
      oid = parts[3] || ''
    }
  }

  return { ...emptyActivation(), smdp, matchingId, oid, confirmationRequired }
}

function matchField(raw: string, patterns: RegExp[]) {
  for (const pattern of patterns) {
    const match = raw.match(pattern)
    if (match?.[1]) return match[1].trim()
  }
  return ''
}

function looksLikeHost(value: string) {
  const host = stripScheme(value)
  return host.includes('.') && /^[A-Za-z0-9.-]+(?::\d+)?$/.test(host)
}

function looksLikeMatchingId(value: string) {
  return /^[A-Za-z0-9._:-]{4,}$/.test(value) && !looksLikeHost(value)
}

function stripScheme(value: string) {
  return value.replace(/^https?:\/\//i, '').replace(/\/+$/, '').trim()
}
