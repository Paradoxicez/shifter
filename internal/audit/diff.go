package audit

import "reflect"

// ChangedFields returns the changed-fields-only subset of (before -> after)
// per D-24. The two output maps land in Entry.Before and Entry.After
// respectively when the caller wants a UPDATE-style diff (CREATE skips
// before-map entirely, ARCHIVE/RESTORE typically also use full row states
// rather than a field-level diff).
//
// EXPLICIT-NULL semantics (Open Q #4):
//
//   - A field that exists in both maps with different values lands in both
//     output diffs with each side's value.
//   - A field added (in `after` but missing from `before`) lands in
//     beforeDiff with value nil — the JSON encoder produces `null`, which
//     a Phase 6 reviewer reads as "this field was previously absent."
//   - A field removed (in `before` but missing from `after`) lands in
//     afterDiff with value nil — same rendering as above, on the after side.
//   - Equal values are ELIDED from both output maps.
//
// The asymmetry between "missing" and "explicitly null" matters for Phase 6
// audit browse: a diff that says `{archived_at: null}` means the field
// changed from a timestamp to NULL (a restore), whereas a diff with no
// archived_at key at all means the field wasn't part of the change at all.
//
// Both output maps are non-nil — they may be empty (no changes), but never
// nil — so the caller can pass them directly to Entry without nil-checks.
// If the caller wants nil maps (CREATE: nil before; archive of empty diff:
// nil after), they assign the result of ChangedFields and then explicitly
// set the unwanted side to nil.
func ChangedFields(before, after map[string]any) (beforeDiff, afterDiff map[string]any) {
	beforeDiff = map[string]any{}
	afterDiff = map[string]any{}

	// Walk `after` first: capture changes + adds.
	for k, av := range after {
		bv, present := before[k]
		if !present {
			// Field was added. Record explicit-null on the before side.
			beforeDiff[k] = nil
			afterDiff[k] = av
			continue
		}
		if !reflect.DeepEqual(bv, av) {
			beforeDiff[k] = bv
			afterDiff[k] = av
		}
	}

	// Walk `before` to catch removals (keys present in before, missing in after).
	for k, bv := range before {
		if _, present := after[k]; present {
			continue // already handled above
		}
		beforeDiff[k] = bv
		afterDiff[k] = nil // explicit null per Open Q #4
	}

	return beforeDiff, afterDiff
}
