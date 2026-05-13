-- Shifter bundled: create ChirpStack database.
-- The postgres superuser (shifter) owns it; no separate role is needed.
-- This file is executed by the timescale/timescaledb image at first boot
-- via the /docker-entrypoint-initdb.d/ mechanism.
CREATE DATABASE chirpstack
    WITH OWNER = shifter
         ENCODING = 'UTF8'
         LC_COLLATE = 'en_US.utf8'
         LC_CTYPE = 'en_US.utf8'
         TEMPLATE = template0;

-- ChirpStack v4 schema migrations require pg_trgm for gin_trgm_ops indexes.
\connect chirpstack
CREATE EXTENSION IF NOT EXISTS pg_trgm;
