import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createBrowserRouter } from 'react-router-dom'
import { ThemeProvider } from '@/components/theme-provider'
import { Toaster } from '@/components/ui/sonner'
import { queryClient } from '@/lib/query-client'
import AuthLayout from '@/routes/_auth'
import RootLayout, { rootLoader } from '@/routes/_root'
import IndexRedirect from '@/routes/index-redirect'

/**
 * Phase 1 router skeleton.
 *
 * Public auth-flow routes (login, install) are children of <AuthLayout />;
 * protected routes (everything else) are children of <RootLayout />, whose
 * `rootLoader` (Plan 11) gates on session presence — 401 → /login redirect.
 *
 * Placeholder elements at /login, /install, /settings will be filled by:
 *   - /login    → Plan 23 (login-ui)
 *   - /install  → Plan 16 (install-wizard-ui)
 *   - /settings → Plan 17 (test-connection) and Plan 11 (account-ui)
 */
const router = createBrowserRouter([
  {
    element: <AuthLayout />,
    children: [
      { path: '/login', element: <div>Login screen — Plan 23</div> },
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
