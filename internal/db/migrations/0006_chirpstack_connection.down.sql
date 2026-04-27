-- 0006_chirpstack_connection.down.sql
DROP TRIGGER IF EXISTS chirpstack_connection_touch ON chirpstack_connection;
DROP TABLE IF EXISTS chirpstack_connection;
DROP TYPE IF EXISTS chirpstack_mode;
