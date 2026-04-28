// Package config — minimal Config struct + Load() stub.
//
// Plan 04 (config-secrets) replaces this with the full viper-driven loader,
// secrets resolver, and Validate() per D-05/D-06/D-07/D-22. Plan 05 only
// needs a Config struct shape with DB, HTTPPort, LogLevel, TLS, and Env so
// the CLI subcommands compile and can call config.Load() during their RunE.
package config
