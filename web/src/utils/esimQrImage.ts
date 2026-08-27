type TransferLike = {
  files?: ArrayLike<File> | null
  items?: ArrayLike<{
    kind?: string
    type?: string
    getAsFile?: () => File | null
    getAsString?: (callback: (value: string) => void) => void
  }> | null
  types?: ArrayLike<string> | null
}

export function transferLooksLikeImageDrop(data: TransferLike | null) {
  if (!data) return false
  if (pickClipboardOrDropImage(data)) return true
  const types = data.types ? Array.from(data.types) : []
  return types.some((type) => type === 'Files' || type.startsWith('image/') || type === 'application/pdf')
}

export function pickClipboardOrDropImage(data: TransferLike | null): File | null {
  if (!data) return null
  const files = data.files ? Array.from(data.files) : []
  const fromFiles = files.find(isActivationMediaFile)
  if (fromFiles) return fromFiles
  const items = data.items ? Array.from(data.items) : []
  for (const item of items) {
    if (isActivationMediaType(item.type || '')) {
      const file = item.getAsFile?.()
      if (file) return file
    }
  }
  return null
}

export async function pickImageFromClipboardData(data: TransferLike | null): Promise<File | null> {
  const direct = pickClipboardOrDropImage(data)
  if (direct) return direct
  const items = data?.items ? Array.from(data.items) : []
  for (const item of items) {
    if (item.type !== 'text/html' || !item.getAsString) continue
    const html = await readTransferString(item)
    const file = fileFromClipboardHtml(html)
    if (file) return file
  }
  return null
}

export async function readImageFromSystemClipboard(): Promise<File | null> {
  const clipboard = (globalThis as typeof globalThis & {
    navigator?: Navigator
  }).navigator?.clipboard
  if (!clipboard || typeof clipboard.read !== 'function') return null
  try {
    const items = await clipboard.read()
    for (const item of items) {
      const type = item.types.find((entry) => isActivationMediaType(entry))
      if (!type) continue
      const blob = await item.getType(type)
      return new File([blob], clipboardFileName(type), { type: blob.type || type })
    }
  } catch {
    return null
  }
  return null
}

export function fileFromClipboardHtml(html: string): File | null {
  const match = html.match(/<img[^>]+src=["'](data:image\/[a-z0-9.+-]+;base64,[^"']+)["']/i)
  if (!match) return null
  return fileFromDataUrl(match[1])
}

function fileFromDataUrl(dataUrl: string): File | null {
  const match = dataUrl.match(/^data:(image\/[a-z0-9.+-]+);base64,([a-z0-9+/]+=*)$/i)
  if (!match) return null
  const type = match[1]
  const binary = atob(match[2])
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i)
  return new File([bytes], clipboardFileName(type), { type })
}

function readTransferString(item: { getAsString?: (callback: (value: string) => void) => void }): Promise<string> {
  return new Promise((resolve) => {
    if (!item.getAsString) {
      resolve('')
      return
    }
    item.getAsString((value) => resolve(value || ''))
  })
}

function clipboardFileName(type: string) {
  const subtype = type.split('/')[1] || 'png'
  return `clipboard.${subtype.split(';')[0] || 'png'}`
}

function isImageType(type: string) {
  return type.startsWith('image/')
}

function isActivationMediaType(type: string) {
  return isImageType(type) || type === 'application/pdf'
}

function isActivationMediaFile(file: File) {
  return isActivationMediaType(file.type) || /\.(png|jpe?g|webp|gif|bmp|heic|heif|pdf)$/i.test(file.name)
}
