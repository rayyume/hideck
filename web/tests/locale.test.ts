import assert from 'node:assert/strict'
import test from 'node:test'
import { applyDocumentLocale, isAppLocale, nextLocale, readStoredLocale, resolveStoredLocale } from '../src/utils/locale.ts'
import { en } from '../src/i18n/en.ts'
import { zhCN } from '../src/i18n/zh-CN.ts'

test('resolveStoredLocale maps aliases', () => {
  assert.equal(resolveStoredLocale('en'), 'en')
  assert.equal(resolveStoredLocale('en-US'), 'en')
  assert.equal(resolveStoredLocale('zh-CN'), 'zh-CN')
  assert.equal(resolveStoredLocale('zh'), 'zh-CN')
  assert.equal(resolveStoredLocale('nope'), 'zh-CN')
  assert.equal(isAppLocale('en'), true)
  assert.equal(isAppLocale('fr'), false)
})

test('readStoredLocale falls back to zh-CN without storage', () => {
  assert.equal(readStoredLocale(null), 'zh-CN')
  assert.equal(nextLocale('zh-CN'), 'en')
  assert.equal(nextLocale('en'), 'zh-CN')
})

test('english catalog covers every Chinese key', () => {
  function walk(left: unknown, right: unknown, path: string) {
    if (typeof left === 'string') {
      assert.equal(typeof right, 'string', path)
      assert.ok(String(right).length > 0, path)
      return
    }
    assert.equal(typeof right, 'object', path)
    for (const key of Object.keys(left as object)) {
      walk((left as Record<string, unknown>)[key], (right as Record<string, unknown>)[key], path ? `${path}.${key}` : key)
    }
  }
  walk(zhCN, en, '')
})

test('applyDocumentLocale sets html lang', () => {
  const root = { lang: '' }
  applyDocumentLocale('en', root)
  assert.equal(root.lang, 'en')
  applyDocumentLocale('zh-CN', root)
  assert.equal(root.lang, 'zh-CN')
})
