---
phase: 01-foundation
plan: 11
type: execute
wave: 8
depends_on: [06, 09]
files_modified:
  - web/src/lib/auth.ts
  - web/src/lib/auth.test.ts
  - web/src/routes/_root.tsx
  - web/src/routes/change-password-dialog.tsx
  - web/src/components/shell/account-menu.tsx
  - web/src/components/account-menu.test.tsx
  - web/src/App.tsx
autonomous: true
requirements:
  - AUTH-05
  - AUTH-06
must_haves:
  truths:
    - "RootLayout's loader fetches /api/account/me, redirects to /login on 401"
    - "AccountMenu hides admin-only items when user.role === 'viewer' (AUTH-06 frontend hiding)"
    - "ChangePasswordDialog uses ResponsiveDialog from Plan 06; submits {current_password, new_password}"
    - "On 422 weak_password, dialog shows inline error with strength tier hint"
    - "On 401 current_password_incorrect, dialog shows inline error 'Current password incorrect'"
    - "Successful change closes dialog and shows sonner toast 'Password changed' (UI-SPEC verbatim)"
    - "auth.ts exports fetchSessionUser, login, logout, changePassword typed wrappers"
  artifacts:
    - path: "web/src/lib/auth.ts"
      provides: "Typed auth client (fetchSessionUser, login, logout, changePassword)"
      contains: "export async function fetchSessionUser"
    - path: "web/src/routes/_root.tsx"
      provides: "RootLayout with loader that gates on session presence (Plan 06 stub replaced)"
      contains: "useLoaderData"
    - path: "web/src/routes/change-password-dialog.tsx"
      provides: "Change password dialog (UI-SPEC dialog anatomy)"
      contains: "ChangePasswordDialog"
    - path: "web/src/components/shell/account-menu.tsx"
      provides: "AccountMenu enhanced with viewer/admin hiding (Plan 06 file replaced)"
      contains: "userRole === 'viewer'"
  key_links:
    - from: "web/src/routes/_root.tsx loader"
      to: "/api/account/me"
      via: "fetchSessionUser → 401 redirect to /login"
      pattern: "redirect.*login"
    - from: "web/src/routes/change-password-dialog.tsx"
      to: "/api/account/password"
      via: "apiFetch POST"
      pattern: "/api/account/password"
---

<objective>
Wire the post-login UI: typed auth client (`fetchSessionUser`, `login`, `logout`, `changePassword`), the RootLayout loader that gates on session presence (returning the user to populate AccountMenu), the change-password dialog using ResponsiveDialog from Plan 06, and the AccountMenu's role-aware hiding (AUTH-06 frontend portion).

Purpose: AUTH-05 frontend (change own password from account menu), AUTH-06 frontend hiding (viewers don't see admin-only controls). Plan 09 backend handlers are consumed here.

Output: After login, RootLayout shows the AccountMenu; clicking "Change password" opens a dialog; on success the dialog closes and a sonner toast fires; viewer users do NOT see admin-only menu items.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-UI-SPEC.md
@01-06-frontend-shell-PLAN.md
@01-09-login-ratelimit-PLAN.md

<interfaces>
Backend API contracts (Plan 09):
- GET /api/account/me — returns { user: { id, email, role, must_change_password } } (200) or 401
- POST /api/auth/login — { email, password } → 200 { user }
- POST /api/auth/logout — 204
- POST /api/account/password — { current_password, new_password } → 200 { ok: true } | 401 | 422

Plan 09 doesn't expose `/api/account/me` yet; this plan adds it (small addition to handlers.go is folded into Plan 11 backend addendum below).

UI-SPEC §"Avatar dropdown menu (Phase 1)":
- "Change password", "Theme" (Light/Dark/System), separator, "Sign out"

UI-SPEC §"Phase 1 copy table" verbatim strings:
- Dialog title: "Change password"
- Submit idle: "Change password"
- Submit loading: "Changing password…"
- Success toast: "Password changed"
- Error: "Current password incorrect" (for 401)
- Strength hint: "At least 12 characters with mixed case, a number, and a symbol."
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1 (backend addendum): GET /api/account/me + AccountInfoHandler</name>
  <files>internal/auth/handlers.go</files>
  <read_first>
    - 01-09-login-ratelimit-PLAN.md (LoginHandler, LoginDeps shape)
    - 01-08-session-manager-PLAN.md (auth.GetUser, auth.User)
  </read_first>
  <action>
1. Append to `internal/auth/handlers.go` a new handler `AccountInfoHandler`:
   ```go
   // AccountInfoHandler returns GET /api/account/me — the post-login user info.
   // Returns 401 if no session.
   func AccountInfoHandler(deps LoginDeps) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           u, ok := GetUser(r.Context(), deps.SessionMgr)
           if !ok {
               writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
               return
           }
           // Hydrate from DB to surface email + must_change_password
           rec, err := deps.Store.getByID(r.Context(), u.ID)
           if err != nil {
               writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
               return
           }
           writeJSON(w, http.StatusOK, map[string]any{
               "user": map[string]any{
                   "id":                   rec.ID,
                   "email":                rec.Email,
                   "role":                 rec.Role,
                   "must_change_password": rec.MustChangePassword,
               },
           })
       }
   }
   ```

2. Add a quick test to `internal/auth/handlers_test.go`:
   ```go
   func TestAccountInfo_ReturnsUser(t *testing.T) {
       f := setupLogin(t)
       res := loginPost(t, f, f.email, f.passwd); res.Body.Close()
       // Need a /me endpoint mounted with AccountInfoHandler — extend setupLogin.
       // (For brevity in this plan: this test is verified through the existing
       // /me mux handler that already calls GetUser; AccountInfoHandler exposes
       // the JSON shape consumed by the frontend.)
   }
   ```
   Skip wiring a separate test if `setupLogin`'s existing `/me` handler covers the assertion path. Add `AccountInfoHandler` mounted at `/api/account/me` in `setupLogin`'s mux.

   Updated handler mounting in `setupLogin`:
   ```go
   mux.Handle("GET /api/account/me", AccountInfoHandler(login))
   ```
   Then add a real test:
   ```go
   func TestAccountInfo_ReturnsUser(t *testing.T) {
       f := setupLogin(t)
       res := loginPost(t, f, f.email, f.passwd); res.Body.Close()

       req, _ := http.NewRequest("GET", f.server.URL+"/api/account/me", nil)
       req.Header.Set("X-Requested-With", "shifter")
       res2, err := f.client.Do(req)
       require.NoError(t, err); defer res2.Body.Close()
       require.Equal(t, 200, res2.StatusCode)

       var body struct {
           User struct {
               Email              string `json:"email"`
               Role               string `json:"role"`
               MustChangePassword bool   `json:"must_change_password"`
           } `json:"user"`
       }
       require.NoError(t, json.NewDecoder(res2.Body).Decode(&body))
       require.Equal(t, f.email, body.User.Email)
       require.Equal(t, "admin", body.User.Role)
       require.False(t, body.User.MustChangePassword, "D-09: wizard admin must_change_password=false")
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/auth -run 'TestAccountInfo_' -race -count=1</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/auth/handlers.go` exports `func AccountInfoHandler(deps LoginDeps) http.HandlerFunc`
    - Handler returns 401 when no session
    - Handler returns 200 with `{ user: { id, email, role, must_change_password } }` when authenticated
    - `TestAccountInfo_ReturnsUser` passes; verifies `must_change_password: false` for the wizard admin (D-09)
  </acceptance_criteria>
  <done>
    Backend `/api/account/me` ready for the frontend loader.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Frontend auth client + RootLayout loader + AccountMenu role-aware</name>
  <files>web/src/lib/auth.ts, web/src/lib/auth.test.ts, web/src/routes/_root.tsx, web/src/components/shell/account-menu.tsx, web/src/components/account-menu.test.tsx, web/src/App.tsx</files>
  <read_first>
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Avatar dropdown menu (Phase 1)" (line 271) — exact menu items
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Phase 1 copy table" (lines 578-606) — verbatim strings
    - .planning/phases/01-foundation/01-RESEARCH.md §"React Router v7 protected route pattern" (lines 1556-1596)
    - 01-06-frontend-shell-PLAN.md (apiFetch, ApiError, ResponsiveShell, AccountMenuProps shape)
  </read_first>
  <behavior>
    - **auth.ts tests:**
      - fetchSessionUser returns the user object on 200
      - fetchSessionUser throws ApiError(401) on 401 — apiFetch already redirects to /login
      - login posts to /api/auth/login with X-Requested-With and returns user
      - changePassword on 422 returns the error body { error: 'weak_password', tier: 'weak' } via thrown ApiError
    - **AccountMenu tests:**
      - When userRole='viewer', the "Change password" item still appears (viewer can self-edit) but admin-only items (none in Phase 1, but the test asserts the structure remains the same for forward compat)
      - When userRole='admin', all items present
      - Verifies the canonical menu items: "Change password", "Theme" (with submenu Light/Dark/System), "Sign out"
  </behavior>
  <action>
1. Create `web/src/lib/auth.ts`:
   ```ts
   import { ApiError, apiFetch } from './api'

   export interface SessionUser {
     id: string
     email: string
     role: 'admin' | 'viewer'
     must_change_password: boolean
   }

   export async function fetchSessionUser(): Promise<SessionUser> {
     const body = await apiFetch<{ user: SessionUser }>('/api/account/me')
     return body.user
   }

   export async function login(email: string, password: string): Promise<SessionUser> {
     const body = await apiFetch<{ user: SessionUser }>('/api/auth/login', {
       method: 'POST',
       body: JSON.stringify({ email, password }),
     })
     return body.user
   }

   export async function logout(): Promise<void> {
     await apiFetch('/api/auth/logout', { method: 'POST' })
   }

   export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
     await apiFetch('/api/account/password', {
       method: 'POST',
       body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
     })
   }

   export { ApiError }
   ```

2. Replace `web/src/lib/auth.test.ts`:
   ```ts
   import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
   import { ApiError } from './api'
   import { changePassword, fetchSessionUser, login } from './auth'

   const mockFetch = (status: number, body: unknown, headers: Record<string, string> = {}) =>
     vi.spyOn(global, 'fetch').mockResolvedValue({
       ok: status >= 200 && status < 300,
       status,
       statusText: 'mock',
       text: async () => JSON.stringify(body),
       headers: new Headers(headers),
     } as unknown as Response)

   describe('auth client', () => {
     afterEach(() => { vi.restoreAllMocks() })

     it('fetchSessionUser returns user object on 200', async () => {
       mockFetch(200, { user: { id: '1', email: 'a@x', role: 'admin', must_change_password: false } })
       const u = await fetchSessionUser()
       expect(u.email).toBe('a@x')
       expect(u.role).toBe('admin')
     })

     it('login sends X-Requested-With header', async () => {
       const fetchSpy = mockFetch(200, { user: { id: '1', email: 'a@x', role: 'admin', must_change_password: false } })
       await login('a@x', 'pw')
       const init = fetchSpy.mock.calls[0][1] as RequestInit
       const headers = new Headers(init.headers)
       expect(headers.get('X-Requested-With')).toBe('shifter')
     })

     it('changePassword throws ApiError on 422', async () => {
       const beforeAssign = window.location.assign
       window.location.assign = vi.fn() as unknown as typeof window.location.assign
       mockFetch(422, { error: 'weak_password', tier: 'weak' })
       await expect(changePassword('cur', 'short')).rejects.toBeInstanceOf(ApiError)
       window.location.assign = beforeAssign
     })
   })
   ```

3. Replace `web/src/routes/_root.tsx`:
   ```tsx
   import { useState } from 'react'
   import { Outlet, redirect, useLoaderData, useRevalidator } from 'react-router-dom'
   import { toast } from 'sonner'
   import { ResponsiveShell } from '@/components/shell/responsive-shell'
   import { fetchSessionUser, logout, type SessionUser } from '@/lib/auth'
   import { ChangePasswordDialog } from './change-password-dialog'

   /**
    * RootLayout loader: fetches the session user; on 401 redirects to /login.
    * Plan 14 will compose this with a first-run-install gate.
    */
   export async function rootLoader({ request }: { request: Request }) {
     try {
       const user = await fetchSessionUser()
       return { user }
     } catch {
       const url = new URL(request.url)
       throw redirect(`/login?next=${encodeURIComponent(url.pathname)}`)
     }
   }

   export default function RootLayout() {
     const { user } = useLoaderData() as { user: SessionUser }
     const revalidator = useRevalidator()
     const [changePwOpen, setChangePwOpen] = useState(false)

     const handleSignOut = async () => {
       try {
         await logout()
       } finally {
         window.location.assign('/login')
       }
     }

     return (
       <>
         <ResponsiveShell
           installDisplayName="Shifter"
           userEmail={user.email}
           userRole={user.role}
           onChangePassword={() => setChangePwOpen(true)}
           onSignOut={handleSignOut}
         />
         <Outlet />
         <ChangePasswordDialog
           open={changePwOpen}
           onOpenChange={setChangePwOpen}
           onSuccess={() => {
             toast.success('Password changed')
             revalidator.revalidate()
           }}
         />
       </>
     )
   }
   ```

4. Create `web/src/routes/change-password-dialog.tsx`:
   ```tsx
   import { useState } from 'react'
   import { Button } from '@/components/ui/button'
   import { Input } from '@/components/ui/input'
   import { Label } from '@/components/ui/label'
   import { Alert, AlertDescription } from '@/components/ui/alert'
   import { ResponsiveDialog } from '@/components/responsive-dialog'
   import { ApiError, changePassword } from '@/lib/auth'

   export interface ChangePasswordDialogProps {
     open: boolean
     onOpenChange: (open: boolean) => void
     onSuccess: () => void
   }

   export function ChangePasswordDialog({ open, onOpenChange, onSuccess }: ChangePasswordDialogProps) {
     const [current, setCurrent] = useState('')
     const [next, setNext] = useState('')
     const [confirm, setConfirm] = useState('')
     const [error, setError] = useState<string | null>(null)
     const [busy, setBusy] = useState(false)

     const reset = () => { setCurrent(''); setNext(''); setConfirm(''); setError(null); setBusy(false) }

     const onSubmit = async (e: React.FormEvent) => {
       e.preventDefault()
       setError(null)
       if (next !== confirm) { setError("New passwords don't match."); return }
       setBusy(true)
       try {
         await changePassword(current, next)
         onSuccess()
         onOpenChange(false)
         reset()
       } catch (err) {
         if (err instanceof ApiError) {
           switch (err.status) {
             case 401: setError('Current password incorrect.'); break
             case 422: setError('Password is too weak. Add length, mixed case, a number, and a symbol.'); break
             default:  setError('Something went wrong. Try again.')
           }
         } else {
           setError('Something went wrong. Try again.')
         }
       } finally {
         setBusy(false)
       }
     }

     return (
       <ResponsiveDialog
         open={open}
         onOpenChange={(v) => { onOpenChange(v); if (!v) reset() }}
         title="Change password"
         footer={
           <>
             <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
             <Button form="change-pw-form" type="submit" disabled={busy}>
               {busy ? 'Changing password…' : 'Change password'}
             </Button>
           </>
         }
       >
         <form id="change-pw-form" onSubmit={onSubmit} className="flex flex-col gap-4">
           {error ? (
             <Alert variant="destructive">
               <AlertDescription>{error}</AlertDescription>
             </Alert>
           ) : null}
           <div className="flex flex-col gap-2">
             <Label htmlFor="cp-current">Current password</Label>
             <Input id="cp-current" type="password" value={current} onChange={(e) => setCurrent(e.target.value)} required />
           </div>
           <div className="flex flex-col gap-2">
             <Label htmlFor="cp-new">New password</Label>
             <Input id="cp-new" type="password" value={next} onChange={(e) => setNext(e.target.value)} required />
             <p className="text-sm text-muted-foreground">At least 12 characters with mixed case, a number, and a symbol.</p>
           </div>
           <div className="flex flex-col gap-2">
             <Label htmlFor="cp-confirm">Confirm new password</Label>
             <Input id="cp-confirm" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required />
           </div>
         </form>
       </ResponsiveDialog>
     )
   }
   ```

5. The `AccountMenu` from Plan 06 is already role-aware in spec — it always shows "Change password" / "Theme" / "Sign out". For Phase 1 there are no admin-only items in the menu (UX-01 specifies items in UI-SPEC §App shell line 271). The test in `account-menu.test.tsx` simply asserts presence of the canonical items.

   Replace `web/src/components/account-menu.test.tsx`:
   ```tsx
   import { render, screen } from '@testing-library/react'
   import userEvent from '@testing-library/user-event'
   import { describe, expect, it } from 'vitest'
   import { AccountMenu } from './shell/account-menu'
   import { ThemeProvider } from './theme-provider'

   function renderMenu(role: 'admin' | 'viewer' = 'admin') {
     return render(
       <ThemeProvider>
         <AccountMenu
           userEmail="alice@example.com"
           userRole={role}
           onChangePassword={() => {}}
           onSignOut={() => {}}
         />
       </ThemeProvider>,
     )
   }

   describe('AccountMenu', () => {
     it('shows Change password, Theme, Sign out for admin', async () => {
       renderMenu('admin')
       const trigger = screen.getByRole('button', { name: /account menu/i })
       await userEvent.click(trigger)
       expect(screen.getByText('Change password')).toBeInTheDocument()
       expect(screen.getByText('Theme')).toBeInTheDocument()
       expect(screen.getByText('Sign out')).toBeInTheDocument()
     })

     it('viewer also sees Change password (AUTH-06: viewers can self-edit)', async () => {
       renderMenu('viewer')
       const trigger = screen.getByRole('button', { name: /account menu/i })
       await userEvent.click(trigger)
       expect(screen.getByText('Change password')).toBeInTheDocument()
     })

     it('shows the email and role in the menu label', async () => {
       renderMenu('viewer')
       const trigger = screen.getByRole('button', { name: /account menu/i })
       await userEvent.click(trigger)
       expect(screen.getByText('alice@example.com')).toBeInTheDocument()
       expect(screen.getByText('viewer')).toBeInTheDocument()
     })
   })
   ```

6. Update `web/src/App.tsx` to use the new loader:
   ```tsx
   import { QueryClientProvider } from '@tanstack/react-query'
   import { RouterProvider, createBrowserRouter } from 'react-router-dom'
   import { ThemeProvider } from '@/components/theme-provider'
   import { Toaster } from '@/components/ui/sonner'
   import { queryClient } from '@/lib/query-client'
   import RootLayout, { rootLoader } from '@/routes/_root'
   import AuthLayout from '@/routes/_auth'
   import IndexRedirect from '@/routes/index-redirect'

   const router = createBrowserRouter([
     {
       element: <AuthLayout />,
       children: [
         { path: '/login',   element: <div>Login screen — Plan 23</div> },
         { path: '/install', element: <div>Install wizard — Plan 16</div> },
       ],
     },
     {
       path: '/',
       element: <RootLayout />,
       loader: rootLoader,
       children: [
         { index: true, element: <IndexRedirect /> },
         { path: 'settings', element: <div>Settings — Plan 17</div> },
       ],
     },
   ])

   export default function App() {
     return (
       <ThemeProvider>
         <QueryClientProvider client={queryClient}>
           <RouterProvider router={router} />
           <Toaster position="top-right" richColors />
         </QueryClientProvider>
       </ThemeProvider>
     )
   }
   ```

7. Run `pnpm test --run` and `pnpm build`.
  </action>
  <verify>
    <automated>cd web && pnpm test:run -- auth account-menu 2>&1 | grep -E '(passed|failed)' | tee /tmp/web-acct.txt && ! grep -q failed /tmp/web-acct.txt && pnpm build</automated>
  </verify>
  <acceptance_criteria>
    - File `web/src/lib/auth.ts` exports `fetchSessionUser`, `login`, `logout`, `changePassword`, `SessionUser` type, re-exports `ApiError`
    - File `web/src/routes/_root.tsx` exports `rootLoader` that calls `fetchSessionUser` and throws `redirect('/login?next=...')` on failure
    - File `web/src/routes/change-password-dialog.tsx` exports `ChangePasswordDialog` using `ResponsiveDialog`
    - Submit button label is exactly "Change password" idle and "Changing password…" loading (UI-SPEC verbatim)
    - Strength helper text is exactly "At least 12 characters with mixed case, a number, and a symbol." (UI-SPEC verbatim)
    - Cancel button is on the LEFT, primary on the RIGHT (UI-SPEC §Dialog Conventions)
    - Tests in `web/src/lib/auth.test.ts` pass (3 tests, X-Requested-With header verified)
    - Tests in `web/src/components/account-menu.test.tsx` pass (3 tests)
    - Command `cd web && pnpm test:run` exits 0 with all suites passing
    - Command `cd web && pnpm build` exits 0
  </acceptance_criteria>
  <done>
    Account menu + change password dialog functional. Plan 16 (install wizard) and Plan 17 (settings) consume the same `_root.tsx` shell. Plan 23 (login) populates the session that triggers the loader.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| browser → /api/account/* | Authenticated session required; CSRF guarded |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-11-01 | Tampering | XSS injects into email render in topbar | mitigate | React auto-escapes `userEmail`; never `dangerouslySetInnerHTML`. ASVS V5. |
| T-11-02 | Information Disclosure | password fields not type="password" | mitigate | All three fields explicitly `type="password"` in JSX. ASVS V2. |
| T-11-03 | Tampering (CSRF) | password change without CSRF guard | mitigate | `apiFetch` adds `X-Requested-With`; backend rejects without it (Plan 09). ASVS V13. |
| T-11-04 | Information Disclosure | password values logged in browser console on error | mitigate | Error handling doesn't `console.log` the password; only ApiError.status is consulted. |
| T-11-05 | Spoofing (UI) | clickjacked dialog | mitigate | Caddy sets `X-Frame-Options: DENY` (Plan 22). ASVS V14. |
</threat_model>

<verification>
- `auth.ts` exports typed wrappers around all 4 endpoints
- RootLayout loader gates on session presence (401 → /login)
- ChangePasswordDialog uses ResponsiveDialog, follows UI-SPEC strings
- AccountMenu shows canonical 3 items (+ Theme submenu); both roles see "Change password"
- Backend `/api/account/me` ready
- All tests pass (auth client + account menu)
</verification>

<success_criteria>
- AUTH-05 frontend complete (change password from account menu)
- AUTH-06 frontend hiding ready (admin-only items will be added with `userRole === 'admin' &&` guards in later phases)
- D-09 verified at `/api/account/me` (must_change_password=false for wizard admin)
- UI-SPEC dialog anatomy followed (Cancel left, primary right; ResponsiveDialog wrapper; navy primary)
- Test Connection result and Settings page (Plan 17) inherit the same patterns
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-11-SUMMARY.md` documenting:
- auth.ts public API
- rootLoader behavior
- ChangePasswordDialog props and copy strings
- AccountMenu role-aware rendering pattern (template for Phase 2+ admin-only items)
</output>
