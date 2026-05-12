-- 0054_audit_vocab_report_template.down.sql
-- Removing enum-like CHECK literals is destructive if rows exist with those values.
-- The down migration restores the pre-0054 CHECK constraints. Any rows written
-- with 'report_template.*' actions or 'report_template' entity type will cause
-- the restore to fail — this is intentional (operator should truncate audit rows
-- first if rolling back is required).
SELECT 1; -- down handled by 0053 table drop covering the data; constraint restore is manual.
