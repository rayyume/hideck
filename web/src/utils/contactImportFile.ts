const CONTACT_FILE_RE = /\.(vcf|vcard|csv|txt)$/i

export function isContactImportFile(file: File) {
  if (CONTACT_FILE_RE.test(file.name)) return true
  const type = (file.type || '').toLowerCase()
  return type.includes('vcard') || type === 'text/csv' || type === 'text/plain' || type === 'text/comma-separated-values'
}

export function dataTransferHasFiles(dt: DataTransfer | null) {
  return !!dt && Array.from(dt.types || []).includes('Files')
}

export function contactImportFilesFromDataTransfer(dt: DataTransfer | null) {
  if (!dt) return []
  return Array.from(dt.files || []).filter(isContactImportFile)
}
