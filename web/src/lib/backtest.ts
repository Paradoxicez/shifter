/**
 * Plan 07-10 — Backtest API client.
 *
 * runBacktest calls POST /api/alerts/backtest and returns the typed
 * BacktestResponse. Used by AddRuleDialog's "Test against last 30 days"
 * button.
 *
 * Auth: ActionAlertRuleCreate (admin only — same as creating a rule).
 * The endpoint is read-only and never writes alert rows (T-07-10-04).
 */

import { apiFetch } from './api'

export type DailyFireBucket = {
  day: string   // YYYY-MM-DD
  count: number
}

export type BacktestResponse = {
  fires_count: number
  daily_fires: DailyFireBucket[]
}

/**
 * Run a backtest for the given anomaly rule kind against the last `days` days
 * of hourly data for the given metering point.
 *
 * @param rule_kind  One of: anomaly_p95 | anomaly_iqr | anomaly_quiet_hour
 * @param mp_id      Metering point UUID (string form)
 * @param days       Number of days to look back (default 30, max 90)
 */
export async function runBacktest(
  rule_kind: string,
  mp_id: string,
  days = 30,
): Promise<BacktestResponse> {
  return apiFetch<BacktestResponse>('/api/alerts/backtest', {
    method: 'POST',
    body: JSON.stringify({ rule_kind, mp_id, days }),
  })
}
