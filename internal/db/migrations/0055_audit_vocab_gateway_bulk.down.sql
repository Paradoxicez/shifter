-- 0055_audit_vocab_gateway_bulk.down.sql
-- Rollback: cannot remove enum values from a CHECK constraint without also
-- dropping rows that use them. Convention from prior migrations: no-op.
SELECT 1;
