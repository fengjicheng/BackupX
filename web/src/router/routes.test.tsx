import { describe, expect, it } from 'vitest'
import { appMenuItems, resolveSelectedMenuKey } from './routes'

describe('route menu metadata', () => {
  it('uses the most specific menu path for nested routes', () => {
    expect(resolveSelectedMenuKey('/settings/notifications')).toBe('/settings/notifications')
    expect(resolveSelectedMenuKey('/storage-targets/google-drive/callback')).toBe(
      '/storage-targets',
    )
    expect(resolveSelectedMenuKey('/admin/api-keys')).toBe('/admin')
  })

  it('keeps the legacy system-info alias selected under settings', () => {
    expect(resolveSelectedMenuKey('/system-info')).toBe('/settings')
  })

  it('defines each menu key once', () => {
    const keys = appMenuItems.map((item) => item.key)
    expect(new Set(keys).size).toBe(keys.length)
  })
})
