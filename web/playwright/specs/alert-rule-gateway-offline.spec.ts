import { test, expect } from '@playwright/test'

/**
 * BUG-01 regression guard: AddRuleDialog sends 'offline_gateway' (not 'gateway_offline').
 * Sending the wrong value causes a DB constraint violation → 500.
 * This spec proves the corrected value reaches the API and the API returns 201.
 */
test.describe('Alert rule — gateway offline kind', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('POST /api/alerts/rules with offline_gateway rule_kind returns 201', async ({ request }) => {
    const res = await request.post('/api/alerts/rules', {
      data: {
        rule_kind: 'offline_gateway',
        severity: 'warning',
        scope_kind: 'global',
        scope_id: null,
        cooldown_seconds: 1800,
      },
    })
    // Must succeed — not 422/500 from DB constraint violation.
    expect(res.status()).toBe(201)

    // Cleanup: delete the rule we just created so future runs start clean.
    const body = await res.json()
    if (body?.id) {
      await request.delete(`/api/alerts/rules/${body.id}`)
    }
  })
})
