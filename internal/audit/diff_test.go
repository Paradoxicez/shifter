package audit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestChangedFields_FieldChanged — value-level update produces a diff with
// matched keys on both sides, equal-valued keys ELIDED.
func TestChangedFields_FieldChanged(t *testing.T) {
	before := map[string]any{"a": 1, "b": 2}
	after := map[string]any{"a": 1, "b": 3}

	bd, ad := ChangedFields(before, after)

	require.Equal(t, map[string]any{"b": 2}, bd, "before diff: only changed key")
	require.Equal(t, map[string]any{"b": 3}, ad, "after diff: only changed key")
}

// TestChangedFields_FieldAdded — Open Q #4 EXPLICIT-NULL: a field present in
// after but missing from before lands as nil on the before-diff side.
func TestChangedFields_FieldAdded(t *testing.T) {
	before := map[string]any{"a": 1}
	after := map[string]any{"a": 1, "b": 3}

	bd, ad := ChangedFields(before, after)

	require.Equal(t, map[string]any{"b": nil}, bd, "before diff: added key has explicit-null")
	require.Equal(t, map[string]any{"b": 3}, ad, "after diff: added key has new value")

	// JSON round-trip: the nil must serialize to JSON null (Phase 6 audit browse).
	bdJSON, err := json.Marshal(bd)
	require.NoError(t, err)
	require.JSONEq(t, `{"b":null}`, string(bdJSON), "explicit-null marshals to JSON null")
}

// TestChangedFields_FieldRemoved — symmetric to "added": a field present in
// before but missing from after lands as nil on the after-diff side.
func TestChangedFields_FieldRemoved(t *testing.T) {
	before := map[string]any{"a": 1, "b": 2}
	after := map[string]any{"a": 1}

	bd, ad := ChangedFields(before, after)

	require.Equal(t, map[string]any{"b": 2}, bd, "before diff: removed key has old value")
	require.Equal(t, map[string]any{"b": nil}, ad, "after diff: removed key has explicit-null")

	adJSON, err := json.Marshal(ad)
	require.NoError(t, err)
	require.JSONEq(t, `{"b":null}`, string(adJSON), "explicit-null marshals to JSON null")
}

// TestChangedFields_NoChange — identical maps yield two empty (non-nil) maps.
// Empty diff is intentional: D-24 + 0016 audit_log accepts re-saves with no
// semantic change so the audit trail records "operator clicked save."
func TestChangedFields_NoChange(t *testing.T) {
	before := map[string]any{"a": 1, "b": "hello"}
	after := map[string]any{"a": 1, "b": "hello"}

	bd, ad := ChangedFields(before, after)

	require.NotNil(t, bd, "before diff is non-nil")
	require.NotNil(t, ad, "after diff is non-nil")
	require.Empty(t, bd, "before diff has no entries")
	require.Empty(t, ad, "after diff has no entries")
}

// TestChangedFields_NilBefore — CREATE-shaped call: caller passes nil before
// map. Result: beforeDiff has explicit-null for every key in after; afterDiff
// has the full after content. Caller typically ignores ChangedFields for
// CREATE and just passes nil/full directly to Entry, but we still want this
// behavior to be sane for the case where CREATE goes through the same diff
// helper.
func TestChangedFields_NilBefore(t *testing.T) {
	var before map[string]any // nil
	after := map[string]any{"name": "alpha", "tag": "x"}

	bd, ad := ChangedFields(before, after)

	require.Equal(t, map[string]any{"name": nil, "tag": nil}, bd)
	require.Equal(t, map[string]any{"name": "alpha", "tag": "x"}, ad)
}

// TestChangedFields_NestedValueEquality — reflect.DeepEqual handles nested
// structures (maps, slices) so a re-save with the same nested object doesn't
// produce a spurious diff.
func TestChangedFields_NestedValueEquality(t *testing.T) {
	before := map[string]any{
		"meta": map[string]any{"vendor": "acrel", "model": "ADW300"},
		"tags": []any{"three-phase", "energy"},
	}
	after := map[string]any{
		"meta": map[string]any{"vendor": "acrel", "model": "ADW300"},
		"tags": []any{"three-phase", "energy"},
	}

	bd, ad := ChangedFields(before, after)

	require.Empty(t, bd, "no nested change → empty before diff")
	require.Empty(t, ad, "no nested change → empty after diff")
}
