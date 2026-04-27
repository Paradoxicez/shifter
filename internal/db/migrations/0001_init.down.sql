-- 0001_init.down.sql
-- Extensions are NOT auto-dropped: dropping `timescaledb` cascades into every
-- hypertable and continuous aggregate (irreversible data loss). Operators who
-- truly need to reset a database should drop the database itself, not roll
-- back this migration.
SELECT 1;
