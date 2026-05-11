-- 0019_import_job.down.sql
DROP INDEX IF EXISTS import_job_row_status_idx;
DROP TABLE IF EXISTS import_job_row CASCADE;

DROP TRIGGER IF EXISTS import_job_touch ON import_job;
DROP INDEX IF EXISTS import_job_status_idx;
DROP INDEX IF EXISTS import_job_owner_idx;
DROP TABLE IF EXISTS import_job CASCADE;

DROP TYPE IF EXISTS import_job_row_status;
DROP TYPE IF EXISTS import_job_status;
