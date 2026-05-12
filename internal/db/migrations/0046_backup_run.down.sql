-- Down: 0046_backup_run
DROP INDEX IF EXISTS backup_run_started_at_idx;
DROP TABLE IF EXISTS backup_run;
