import { QueryClientProvider } from '@tanstack/react-query'
import { lazy, Suspense } from 'react'
import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { ThemeProvider } from '@/components/theme-provider'
import { Toaster } from '@/components/ui/sonner'
import { queryClient } from '@/lib/query-client'
import AuthLayout from '@/routes/_auth'
import RootLayout, { rootLoader } from '@/routes/_root'
import IndexRedirect from '@/routes/index-redirect'

const InstallWizard = lazy(() => import('@/routes/install'))
const SettingsPage = lazy(() => import('@/routes/settings'))
const LoginScreen = lazy(() => import('@/routes/login'))
const SitesPage = lazy(() => import('@/routes/sites'))
const SiteDetailPage = lazy(() => import('@/routes/sites/$id'))
const DevicesPage = lazy(() => import('@/routes/devices'))
const MeteringPointDetailPage = lazy(() => import('@/routes/metering-points/$id'))
const ProfilesPage = lazy(() => import('@/routes/profiles'))
const ProfileEditorRoute = lazy(() => import('@/routes/profiles/$id'))

/**
 * Phase 1 router skeleton.
 *
 * Public auth-flow routes (login, install) are children of <AuthLayout />;
 * protected routes (everything else) are children of <RootLayout />, whose
 * `rootLoader` (Plan 11 + Plan 16) gates on session presence (with an
 * install-state pre-check) — 401 → /login redirect; install incomplete →
 * /install redirect.
 */
const router = createBrowserRouter([
  {
    element: <AuthLayout />,
    children: [
      {
        path: '/login',
        element: (
          <Suspense fallback={null}>
            <LoginScreen />
          </Suspense>
        ),
      },
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
      {
        path: 'sites',
        element: (
          <Suspense fallback={null}>
            <SitesPage />
          </Suspense>
        ),
      },
      {
        path: 'sites/:id',
        element: (
          <Suspense fallback={null}>
            <SiteDetailPage />
          </Suspense>
        ),
      },
      {
        path: 'devices',
        element: (
          <Suspense fallback={null}>
            <DevicesPage />
          </Suspense>
        ),
      },
      {
        path: 'metering-points/:id',
        element: (
          <Suspense fallback={null}>
            <MeteringPointDetailPage />
          </Suspense>
        ),
      },
      {
        path: 'profiles',
        element: (
          <Suspense fallback={null}>
            <ProfilesPage />
          </Suspense>
        ),
      },
      {
        path: 'profiles/new',
        element: (
          <Suspense fallback={null}>
            <ProfileEditorRoute />
          </Suspense>
        ),
      },
      {
        path: 'profiles/:id',
        element: (
          <Suspense fallback={null}>
            <ProfileEditorRoute />
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
