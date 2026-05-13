/**
 * Typed fetch wrapper used by every SPA → /api/* call.
 *
 * - Always sends the X-Requested-With: shifter header. The backend
 *   (Plan 11) requires this header on state-changing requests as the
 *   CSRF mitigation per RESEARCH §Security Domain. SameSite=Lax cookies
 *   plus this custom header is the canonical pattern.
 * - On 401, redirects the SPA to /login?next=<current path> so the
 *   user re-authenticates and lands back where they were.
 * - JSON-only: requests with a body get Content-Type: application/json
 *   if not already set; responses are JSON.parse()'d.
 */

export class ApiError extends Error {
  status: number
  body: unknown
  constructor(status: number, message: string, body?: unknown) {
    super(message)
    this.status = status
    this.body = body
  }
}

export async function apiFetch<T = unknown>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  headers.set('X-Requested-With', 'shifter')
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')

  const res = await fetch(path, { ...init, headers, credentials: 'same-origin' })
  const text = await res.text()
  // Go's http.Error() writes text/plain bodies on error paths. JSON.parse
  // would throw and lose the status code, so parse defensively and fall
  // back to the raw text when the body is not JSON.
  let body: unknown = null
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = text
    }
  }

  if (!res.ok) {
    if (res.status === 401) {
      // Plan 11 wires the post-401 redirect once the login route exists.
      window.location.assign(`/login?next=${encodeURIComponent(window.location.pathname)}`)
    }
    const message =
      (body && typeof body === 'object' && 'error' in body && typeof body.error === 'string'
        ? body.error
        : typeof body === 'string' && body.trim().length > 0
        ? body.trim()
        : null) ?? `${res.status} ${res.statusText}`
    throw new ApiError(res.status, message, body)
  }
  return body as T
}
