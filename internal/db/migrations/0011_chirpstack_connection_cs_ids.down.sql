-- 0011_chirpstack_connection_cs_ids.down.sql
ALTER TABLE chirpstack_connection
    DROP COLUMN IF EXISTS cs_application_id,
    DROP COLUMN IF EXISTS cs_tenant_id;
