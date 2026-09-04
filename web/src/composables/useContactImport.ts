import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { phoneContactsService } from '../services/phone-contacts'
import { contactImportFilesFromDataTransfer, dataTransferHasFiles } from '../utils/contactImportFile'

type ContactImportOptions = Readonly<{
  deviceId: () => string | undefined
  reloadContacts: (options?: Readonly<{ fresh?: boolean }>) => Promise<unknown>
  onComplete?: () => void
}>

type ImportTotals = {
  imported: number
  skipped: number
  failed: number
  lastError: string
}

const MESSAGE_Z_INDEX = 5100
const INVALID_FILE_MESSAGE = '请拖入 vcf 或 csv（iOS、Google、三星、小米、华为、OPPO、vivo 通讯录导出）'

function errorMessage(error: unknown, fallback: string) {
  const responseMessage = (error as { response?: { data?: { message?: unknown } } })
    ?.response?.data?.message
  if (typeof responseMessage === 'string' && responseMessage.trim()) return responseMessage.trim()
  return error instanceof Error && error.message ? error.message : fallback
}

function successMessage(totals: ImportTotals) {
  return `已导入 ${totals.imported} 个号码`
    + (totals.skipped ? `，跳过 ${totals.skipped} 条` : '')
    + (totals.failed ? `，${totals.failed} 个文件失败` : '')
}

export function useContactImport(options: ContactImportOptions) {
  const importing = ref(false)
  const dropActive = ref(false)
  let dropDepth = 0

  function resetDropState() {
    dropDepth = 0
    dropActive.value = false
  }

  function onDragEnter(event: DragEvent) {
    if (!dataTransferHasFiles(event.dataTransfer) || importing.value) return
    event.preventDefault()
    dropDepth += 1
    dropActive.value = true
  }

  function onDragOver(event: DragEvent) {
    if (!dataTransferHasFiles(event.dataTransfer) || importing.value) return
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
  }

  function onDragLeave(event: DragEvent) {
    if (!dataTransferHasFiles(event.dataTransfer)) return
    event.preventDefault()
    dropDepth = Math.max(0, dropDepth - 1)
    if (dropDepth === 0) dropActive.value = false
  }

  async function importFiles(files: File[]) {
    if (!files.length || importing.value) return
    importing.value = true
    const totals: ImportTotals = { imported: 0, skipped: 0, failed: 0, lastError: '' }
    try {
      for (const file of files) await importFile(file, totals)
      await finishImport(totals)
    } finally {
      importing.value = false
    }
  }

  async function importFile(file: File, totals: ImportTotals) {
    try {
      const result = await phoneContactsService.importFile(file, options.deviceId())
      totals.imported += result.imported
      totals.skipped += result.skipped
    } catch (error) {
      totals.failed += 1
      totals.lastError = errorMessage(error, '导入失败')
    }
  }

  async function finishImport(totals: ImportTotals) {
    if (!totals.imported && !totals.skipped) {
      ElMessage.error({ message: totals.lastError || '文件里没有识别到联系人。请导出 vcf 或 csv', zIndex: MESSAGE_Z_INDEX })
      return
    }
    options.onComplete?.()
    try {
      await options.reloadContacts({ fresh: true })
    } catch (error) {
      ElMessage.error({
        message: `${successMessage(totals)}，但列表刷新失败：${errorMessage(error, '请重新加载联系人')}`,
        zIndex: MESSAGE_Z_INDEX
      })
      return
    }
    ElMessage.success({ message: successMessage(totals), zIndex: MESSAGE_Z_INDEX })
  }

  async function onDrop(event: DragEvent) {
    event.preventDefault()
    resetDropState()
    const files = contactImportFilesFromDataTransfer(event.dataTransfer)
    if (!files.length) {
      ElMessage.error({ message: INVALID_FILE_MESSAGE, zIndex: MESSAGE_Z_INDEX })
      return
    }
    await importFiles(files)
  }

  async function onImportFile(event: Event) {
    const input = event.target as HTMLInputElement
    const files = Array.from(input.files || [])
    input.value = ''
    await importFiles(files)
  }

  return {
    importing,
    dropActive,
    resetDropState,
    onDragEnter,
    onDragOver,
    onDragLeave,
    onDrop,
    onImportFile
  }
}
