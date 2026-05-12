import { QueryClientProvider } from '@tanstack/react-query'
import { lazy, Suspense } from 'react'
import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { ThemeProvider } from '@/components/theme-provider'
import { Toaster } from '@/components/ui/sonner'
import { queryClient } from '@/lib/query-client'
// Leaflet CSS shim — MUST be imported before any component that uses Leaflet.
// Three CSS imports loaded as a side effect so tiles + popups + cluster icons render.
// Pitfall #4 from RESEARCH: without this, map shows as a grey box.
import './lib/leaflet-css'
import AuthLayout from '@/routes/_auth'
import RootLayout, { rootLoader } from '@/routes/_root'

const DashboardPage = lazy(() => import('@/routes/dashboard'))
const InstallWizard = lazy(() => import('@/routes/install'))
const SettingsPage = lazy(() => import('@/routes/settings'))
const LoginScreen = lazy(() => import('@/routes/login'))
const SitesPage = lazy(() => import('@/routes/sites'))
const SiteDetailPage = lazy(() => import('@/routes/sites/$id'))
const DevicesPage = lazy(() => import('@/routes/devices'))
const DeviceDetailPage = lazy(() => import('@/routes/devices/$id'))
const GatewaysPage = lazy(() => import('@/routes/gateways'))
const GatewayDetailPage = lazy(() => import('@/routes/gateways/$id'))
const MeteringPointDetailPage = lazy(() => import('@/routes/metering-points/$id'))
const ProfilesPage = lazy(() => import('@/routes/profiles'))
const ProfileEditorRoute = lazy(() => import('@/routes/profiles/$id'))
const AdminImportsPage = lazy(() => import('@/routes/admin/imports'))
const ImportJobDetailPage = lazy(() => import('@/routes/admin/imports/$jobId'))
const MapPage = lazy(() => import('@/routes/map'))
const ReportsPage = lazy(() => import('@/routes/reports'))
const UsersPage = lazy(() => import('@/routes/settings/users'))

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
    id: 'root',
    path: '/',
    element: <RootLayout />,
    loader: rootLoader,
    children: [
      {
        index: true,
        element: (
          <Suspense fallback={null}>
            <DashboardPage />
          </Suspense>
        ),
      },
      {
        path: 'settings',
        element: (
          <Suspense fallback={null}>
            <SettingsPage />
          </Suspense>
        ),
      },
      {
        path: 'settings/users',
        element: (
          <Suspense fallback={null}>
            <UsersPage />
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
        path: 'devices/:id',
        element: (
          <Suspense fallback={null}>
            <DeviceDetailPage />
          </Suspense>
        ),
      },
      {
        path: 'gateways',
        element: (
          <Suspense fallback={null}>
            <GatewaysPage />
          </Suspense>
        ),
      },
      {
        path: 'gateways/:id',
        element: (
          <Suspense fallback={null}>
            <GatewayDetailPage />
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
      {
        path: 'admin/imports',
        element: (
          <Suspense fallback={null}>
            <AdminImportsPage />
          </Suspense>
        ),
      },
      {
        path: 'admin/imports/:jobId',
        element: (
          <Suspense fallback={null}>
            <ImportJobDetailPage />
          </Suspense>
        ),
      },
      {
        path: 'map',
        element: (
          <Suspense fallback={null}>
            <MapPage />
          </Suspense>
        ),
      },
      {
        path: 'reports',
        element: (
          <Suspense fallback={null}>
            <ReportsPage />
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
