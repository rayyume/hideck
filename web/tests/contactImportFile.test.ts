import assert from 'node:assert/strict'
import test from 'node:test'
import {
  contactImportFilesFromDataTransfer,
  dataTransferHasFiles,
  isContactImportFile
} from '../src/utils/contactImportFile.ts'

test('accepts common phone and Google contact exports', () => {
  assert.equal(isContactImportFile(new File(['BEGIN:VCARD'], 'xiaomi.vcf')), true)
  assert.equal(isContactImportFile(new File(['姓名,手机'], 'huawei.csv', { type: 'text/csv' })), true)
  assert.equal(isContactImportFile(new File(['a'], 'oppo.vcard')), true)
  assert.equal(isContactImportFile(new File(['BEGIN:VCARD'], 'ios.vcf', { type: 'text/x-vcard' })), true)
  assert.equal(isContactImportFile(new File(['Name,Phone 1 - Value'], 'google.csv')), true)
  assert.equal(isContactImportFile(new File(['Name,Mobile Phone'], 'samsung.csv')), true)
  assert.equal(isContactImportFile(new File(['BEGIN:VCARD'], 'vivo.vcf')), true)
  assert.equal(isContactImportFile(new File(['a'], 'notes.txt', { type: 'text/plain' })), true)
  assert.equal(isContactImportFile(new File(['a'], 'photo.jpg')), false)
})

test('filters dropped files to contact exports', () => {
  assert.equal(dataTransferHasFiles(null), false)
  const dt = {
    types: ['Files'],
    files: [
      new File(['BEGIN:VCARD'], 'contacts.vcf'),
      new File(['x'], 'readme.md')
    ]
  } as unknown as DataTransfer
  assert.equal(dataTransferHasFiles(dt), true)
  const files = contactImportFilesFromDataTransfer(dt)
  assert.equal(files.length, 1)
  assert.equal(files[0].name, 'contacts.vcf')
})
