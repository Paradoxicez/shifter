-- 0004_install_state.down.sql
DROP TRIGGER IF EXISTS install_state_touch ON install_state;
DROP TABLE IF EXISTS install_state;
