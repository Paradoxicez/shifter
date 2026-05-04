/**
 * Client-side RFC 6901 JSON-pointer leaf walker.
 *
 * Mirrors the semantics of internal/profile/handlers.go's flattenJSON helper
 * so the mapping editor's left pane can render a clickable JSON tree even
 * BEFORE a profile has been saved (the backend's POST /decoded-sample
 * endpoint is path-scoped to {id} which doesn't exist in "new" mode). Both
 * implementations cap leaf count to defuse paste-bomb DoS (T-02-14-03).
 *
 * Output shape matches `DecodedSample` in lib/profiles.ts:
 *   { leaves: [{ json_pointer, value }], truncated? }
 */

export interface FlattenedLeaf {
  json_pointer: string
  value: unknown
}

export interface FlattenResult {
  leaves: FlattenedLeaf[]
  truncated: boolean
}

/**
 * MAX_LEAVES caps the leaf count returned by flattenJSON. Aligned with
 * internal/profile/handlers.go's maxDecodedSampleLeaves (200) so a paste
 * that's "too big" client-side is also "too big" server-side after save.
 */
export const MAX_LEAVES = 200

/**
 * flattenJSON walks `val` and returns every leaf (non-container) as
 * `{ json_pointer, value }`. Containers are walked recursively; arrays use
 * `/N` indexing per RFC 6901. Caps at MAX_LEAVES; deeper trees set
 * `truncated: true`.
 */
export function flattenJSON(val: unknown): FlattenResult {
  const leaves: FlattenedLeaf[] = []
  const state = { truncated: false }
  walk('', val, leaves, state)
  return { leaves, truncated: state.truncated }
}

function walk(
  prefix: string,
  val: unknown,
  leaves: FlattenedLeaf[],
  state: { truncated: boolean },
): void {
  if (state.truncated) return

  if (val !== null && typeof val === 'object') {
    if (Array.isArray(val)) {
      for (let i = 0; i < val.length; i++) {
        if (state.truncated) return
        walk(`${prefix}/${i}`, val[i], leaves, state)
      }
      return
    }
    for (const k of Object.keys(val as Record<string, unknown>)) {
      if (state.truncated) return
      walk(`${prefix}/${escapeJSONPointer(k)}`, (val as Record<string, unknown>)[k], leaves, state)
    }
    return
  }

  if (leaves.length >= MAX_LEAVES) {
    state.truncated = true
    return
  }
  leaves.push({ json_pointer: prefix, value: val })
}

/**
 * escapeJSONPointer applies RFC 6901 §4 reverse mapping: ~ → ~0, / → ~1.
 * Order matters: ~ MUST be encoded BEFORE / so "/" inside a key ends up as
 * ~1 rather than ~01.
 */
function escapeJSONPointer(token: string): string {
  return token.replace(/~/g, '~0').replace(/\//g, '~1')
}

/**
 * tryParseJSON parses `text` and returns the parsed value or `null` on
 * error. Used by the mapping editor's left-pane textarea to decide whether
 * to render the JSON tree or render an inline "invalid JSON" hint.
 *
 * Empty/whitespace input returns `null` — the editor uses that to show the
 * empty-state hint copy from UI-SPEC ("Paste sample decoded JSON above").
 */
export function tryParseJSON(text: string): unknown | null {
  const trimmed = text.trim()
  if (!trimmed) return null
  try {
    return JSON.parse(trimmed)
  } catch {
    return null
  }
}
