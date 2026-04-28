// Package web embeds the Vite production bundle into the Shifter binary.
//
// This file lives at the repo root's web/ directory because go:embed
// patterns cannot use ../ — they must point at descendants of the source
// file's containing directory. The dist/ subtree (Vite output) is therefore
// addressable as `all:dist` from here.
//
// internal/http/spa.go consumes Dist via SPAHandlerFS(fs.FS); the production
// SPAHandler() in that package strips the "dist" prefix via fs.Sub.
//
// The `all:` prefix is mandatory per RESEARCH §Pattern 8 — without it,
// go:embed silently excludes dotfiles (e.g. .vite/manifest.json) and the
// bundle would ship missing pieces (T-19-03 mitigation).
//
// Build prerequisite: `pnpm build` must run before `go build` for production
// so dist/ contains real artifacts. The .gitkeep placeholder lets
// `go:embed all:dist` compile in CI before the frontend build runs.
package web

import "embed"

// Dist is the embedded Vite production bundle. Root is the dist/ directory;
// callers should fs.Sub it to "dist" to expose index.html at the FS root.
//
//go:embed all:dist
var Dist embed.FS
