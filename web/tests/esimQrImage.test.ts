import assert from 'node:assert/strict'
import test from 'node:test'
import {
  fileFromClipboardHtml,
  pickClipboardOrDropImage,
  transferLooksLikeImageDrop
} from '../src/utils/esimQrImage'

test('picks an image from drop files or clipboard items', () => {
  const image = new File([new Uint8Array([1, 2, 3])], 'qr.png', { type: 'image/png' })
  const note = new File([new Uint8Array([1])], 'note.txt', { type: 'text/plain' })
  assert.equal(pickClipboardOrDropImage({ files: [note, image] })?.name, 'qr.png')
  assert.equal(pickClipboardOrDropImage({
    files: [],
    items: [{ type: 'image/jpeg', getAsFile: () => image }]
  })?.name, 'qr.png')
  const pdf = new File([new Uint8Array([1])], 'voxi.pdf', { type: 'application/pdf' })
  assert.equal(pickClipboardOrDropImage({ files: [note, pdf] })?.name, 'voxi.pdf')
  assert.equal(pickClipboardOrDropImage({ files: [note] }), null)
  assert.equal(transferLooksLikeImageDrop({ types: ['Files'] }), true)
  assert.equal(transferLooksLikeImageDrop({ types: ['application/pdf'] }), true)
  assert.equal(transferLooksLikeImageDrop({ types: ['image/png'] }), true)
  assert.equal(transferLooksLikeImageDrop({ types: ['text/plain'] }), false)
})

test('extracts a data-URL image from pasted HTML', () => {
  const file = fileFromClipboardHtml(
    '<html><body><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="></body></html>'
  )
  assert.ok(file)
  assert.equal(file?.type, 'image/png')
  assert.ok((file?.size || 0) > 0)
})
