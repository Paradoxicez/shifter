import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { changePassword, fetchSessionUser, login, logout } from './auth'

const mockFetch = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  vi.spyOn(global, 'fetch').mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: 'mock',
    text: async () => (body === null ? '' : JSON.stringify(body)),
    headers: new Headers(headers),
  } as unknown as Response)

// jsdom 29 makes window.location.assign non-configurable. Replace the whole
// location object with a stub via Object.defineProperty (configurable:true) so
// the spy can intercept assign() calls without redefining a sealed accessor.
function stubLocationAssign() {
  const fn = vi.fn()
  Object.defineProperty(window, 'location', {
    configurable: true,
    writable: true,
    value: { ...window.location, assign: fn, pathname: window.location.pathname },
  })
  return fn
}

describe('auth client (Plan 11)', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('fetchSessionUser returns the user object on 200', async () => {
    mockFetch(200, {
      user: { id: '1', email: 'a@x', role: 'admin', must_change_password: false },
    })
    const u = await fetchSessionUser()
    expect(u.email).toBe('a@x')
    expect(u.role).toBe('admin')
    expect(u.must_change_password).toBe(false)
  })

  it('fetchSessionUser throws ApiError on 401 (apiFetch handles redirect)', async () => {
    stubLocationAssign()
    mockFetch(401, { error: 'unauthorized' })
    await expect(fetchSessionUser()).rejects.toBeInstanceOf(ApiError)
  })

  it('login posts to /api/auth/login with X-Requested-With and returns user', async () => {
    const fetchSpy = mockFetch(200, {
      user: { id: '1', email: 'a@x', role: 'admin', must_change_password: false },
    })
    const u = await login('a@x', 'pw')
    expect(u.email).toBe('a@x')
    expect(fetchSpy).toHaveBeenCalledTimes(1)
    const [path, init] = fetchSpy.mock.calls[0]
    expect(path).toBe('/api/auth/login')
    const headers = new Headers((init as RequestInit).headers)
    expect(headers.get('X-Requested-With')).toBe('shifter')
    expect((init as RequestInit).method).toBe('POST')
  })

  it('logout posts to /api/auth/logout (returns void on 204)', async () => {
    const fetchSpy = mockFetch(204, null)
    await logout()
    const [path, init] = fetchSpy.mock.calls[0]
    expect(path).toBe('/api/auth/logout')
    expect((init as RequestInit).method).toBe('POST')
  })

  it('changePassword sends current_password + new_password and throws ApiError on 422', async () => {
    const fetchSpy = mockFetch(422, { error: 'weak_password', tier: 'weak' })
    let caught: unknown
    try {
      await changePassword('cur', 'short')
    } catch (e) {
      caught = e
    }
    expect(caught).toBeInstanceOf(ApiError)
    expect((caught as ApiError).status).toBe(422)
    const [path, init] = fetchSpy.mock.calls[0]
    expect(path).toBe('/api/account/password')
    expect((init as RequestInit).method).toBe('POST')
    const sentBody = JSON.parse((init as RequestInit).body as string)
    expect(sentBody).toEqual({ current_password: 'cur', new_password: 'short' })
  })

  it('changePassword on 401 throws ApiError(401) with current_password_incorrect body', async () => {
    stubLocationAssign()
    mockFetch(401, { error: 'current_password_incorrect' })
    let caught: unknown
    try {
      await changePassword('wrong', 'NewPasswordAbc!1234')
    } catch (e) {
      caught = e
    }
    expect(caught).toBeInstanceOf(ApiError)
    expect((caught as ApiError).status).toBe(401)
  })
})
