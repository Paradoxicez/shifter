-- 0029_retention_config.up.sql
-- Singleton retention configuration (DATA-13 + D-09).
--
-- One row, id always 1, columns store the desired retention windows for raw
-- measurement and each CAGG level. NULL = forever (D-09 yearly default).
--
-- Plan 05-02 seeds this row inside the install FinishSetup transaction with
-- the D-09 defaults. Plan 05-11 exposes a Settings UI that PATCH-updates the
-- row and (in plan 05-11 Task 2) reconciles the actual TimescaleDB retention
-- policies to match.

CREATE TABLE retention_config (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  raw_days     INTEGER NOT NULL CHECK (raw_days BETWEEN 30 AND 365),
  hourly_days  INTEGER NOT NULL CHECK (hourly_days BETWEEN 180 AND 1825),
  daily_days   INTEGER NOT NULL CHECK (daily_days BETWEEN 365 AND 7300),
  monthly_days INTEGER NOT NULL CHECK (monthly_days BETWEEN 1825 AND 18250),
  yearly_days  INTEGER,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE retention_config IS 'Singleton retention windows for measurement + CAGGs (D-09 / DATA-13). NULL yearly_days = forever.';
COMMENT ON COLUMN retention_config.raw_days IS 'Raw measurement retention in days. UI-SPEC range 30..365. Default 90 (D-09).';
COMMENT ON COLUMN retention_config.hourly_days IS 'measurement_hourly retention. UI-SPEC range 180..1825 (6mo..5y). Default 365 (D-09).';
COMMENT ON COLUMN retention_config.daily_days IS 'measurement_daily retention. UI-SPEC range 365..7300 (1y..20y). Default 1825 (D-09).';
COMMENT ON COLUMN retention_config.monthly_days IS 'measurement_monthly retention. UI-SPEC range 1825..18250 (5y..50y). Default 7300 (D-09).';
COMMENT ON COLUMN retention_config.yearly_days IS 'measurement_yearly retention. NULL = forever (D-09).';
