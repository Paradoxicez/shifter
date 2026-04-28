import { QueryClientProvider } from '@tanstack/react-query'
import { Suspense, lazy } from 'react'
import { RouterProvider, createBrowserRouter } from 'react-router-dom'
import { ThemeProvider } from '@/components/theme-provider'
import { Toaster } from '@/components/ui/sonner'
import { queryClient } from '@/lib/query-client'
import AuthLayout from '@/routes/_auth'
import RootLayout, { rootLoader } from '@/routes/_root'
import IndexRedirect from '@/routes/index-redirect'

const InstallWizard = lazy(() => import('@/routes/install'))
const SettingsPage = lazy(() => import('@/routes/settings'))

/**
 * Phase 1 router skeleton.
 *
 * Public auth-flow routes (login, install) are children of <AuthLayout />;
 * protected routes (everything else) are children of <RootLayout />, whose
 * `rootLoader` (Plan 11 + Plan 16) gates on session presence (with an
 * install-state pre-check) — 401 → /login redirect; install incomplete →
 * /install redirect.
 *
 * Placeholder elements at /login, /settings will be filled by:
 *   - /login    → Plan 23 (login-ui)
 *   - /settings → Plan 17 (test-connection) and Plan 11 (account-ui)
 */
const router = createBrowserRouter([
  {
    element: <AuthLayout />,
    children: [
      { path: '/login', element: <div>Login screen — Plan 23</div> },
      {
        path: '/install',
        element: (
          <Suspense fallback={null}>
            <InstallWizard />
          </Suspense>
        ),
      },
    ],
  },
  {
    path: '/',
    element: <RootLayout />,
    loader: rootLoader,
    children: [
      { index: true, element: <IndexRedirect /> },
      {
        path: 'settings',
        element: (
          <Suspense fallback={null}>
            <SettingsPage />
          </Suspense>
        ),
      },
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
