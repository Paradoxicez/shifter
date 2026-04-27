# Shifter

Self-hosted LoRaWAN water and electricity monitoring platform. Wraps ChirpStack as the LoRaWAN backend; presents real-time and historical consumption through a modern, minimal dashboard. Single-tenant per install.

## Quick Start

```bash
just bootstrap   # install dev tools (air, sqlc, mockgen) + pnpm deps
just dev         # run Go backend (via air) + Vite SPA in parallel
```

> Plan 24 will replace this stub with the full README (architecture overview, deployment modes, configuration, etc.).
