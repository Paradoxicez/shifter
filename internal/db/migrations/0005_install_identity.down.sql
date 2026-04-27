-- 0005_install_identity.down.sql
DROP TRIGGER IF EXISTS install_identity_touch ON install_identity;
DROP TABLE IF EXISTS install_identity;
DROP TYPE IF EXISTS units_system;
