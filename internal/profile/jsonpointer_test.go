package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResolve_Root_EmptyPtr — RFC 6901 §5: empty pointer addresses the whole
// document.
func TestResolve_Root_EmptyPtr(t *testing.T) {
	doc := map[string]any{"a": 1}
	got, ok := Resolve(doc, "")
	require.True(t, ok)
	require.Equal(t, doc, got)
}

// TestResolve_NestedMap — canonical happy path; multi-segment pointer walks
// through nested maps.
func TestResolve_NestedMap(t *testing.T) {
	doc := map[string]any{
		"a": map[string]any{
			"b": 42,
		},
	}
	got, ok := Resolve(doc, "/a/b")
	require.True(t, ok)
	require.Equal(t, 42, got)
}

// TestResolve_ArrayIndex — decimal array index addresses a slice element.
func TestResolve_ArrayIndex(t *testing.T) {
	doc := map[string]any{
		"x": []any{1, 2, 3},
	}
	got, ok := Resolve(doc, "/x/1")
	require.True(t, ok)
	require.Equal(t, 2, got)
}

// TestResolve_OutOfBoundsIndex — array index beyond len returns (nil, false);
// MUST NOT panic.
func TestResolve_OutOfBoundsIndex(t *testing.T) {
	doc := map[string]any{"x": []any{1}}
	got, ok := Resolve(doc, "/x/5")
	require.False(t, ok)
	require.Nil(t, got)
}

// TestResolve_MissingKey — map lookup miss returns (nil, false).
func TestResolve_MissingKey(t *testing.T) {
	doc := map[string]any{"a": 1}
	got, ok := Resolve(doc, "/nope")
	require.False(t, ok)
	require.Nil(t, got)
}

// TestResolve_EscapedTilde — RFC 6901 §3: ~0 must unescape to ~ AFTER ~1
// has been processed.
func TestResolve_EscapedTilde(t *testing.T) {
	doc := map[string]any{"with~key": 1}
	got, ok := Resolve(doc, "/with~0key")
	require.True(t, ok)
	require.Equal(t, 1, got)
}

// TestResolve_EscapedSlash — RFC 6901 §3: ~1 unescapes to /.
func TestResolve_EscapedSlash(t *testing.T) {
	doc := map[string]any{"with/key": 1}
	got, ok := Resolve(doc, "/with~1key")
	require.True(t, ok)
	require.Equal(t, 1, got)
}

// TestResolve_InvalidNoSlash — pointer that does not start with "/" (and is
// non-empty) is rejected.
func TestResolve_InvalidNoSlash(t *testing.T) {
	doc := map[string]any{"foo": 1}
	got, ok := Resolve(doc, "foo")
	require.False(t, ok)
	require.Nil(t, got)
}

// TestResolve_EscapeOrderMatters — RFC 6901 §4 requires ~1 to be processed
// BEFORE ~0. The pointer "/~01" must address the key "~1", not "/1".
// (Reverse order would unescape ~0→~, then ~1→/, yielding "/1".)
func TestResolve_EscapeOrderMatters(t *testing.T) {
	doc := map[string]any{"~1": "tilde-one"}
	got, ok := Resolve(doc, "/~01")
	require.True(t, ok)
	require.Equal(t, "tilde-one", got)
}

// TestResolve_WalkThroughLeaf — pointer steps past a non-container value
// (number/string/bool) → (nil, false). Defensive.
func TestResolve_WalkThroughLeaf(t *testing.T) {
	doc := map[string]any{"a": 1}
	got, ok := Resolve(doc, "/a/b")
	require.False(t, ok)
	require.Nil(t, got)
}

// TestResolve_NilDoc — Resolving against a nil document returns (nil, false)
// for any non-empty pointer; the empty pointer returns nil with ok=true
// (the whole document IS nil).
func TestResolve_NilDoc(t *testing.T) {
	got, ok := Resolve(nil, "/foo")
	require.False(t, ok)
	require.Nil(t, got)

	got, ok = Resolve(nil, "")
	require.True(t, ok)
	require.Nil(t, got)
}

// TestResolve_NegativeIndex — negative array indices are NOT supported by
// RFC 6901 (the "-" pseudo-token addresses one past the end and is reserved
// for JSON Patch); returns (nil, false).
func TestResolve_NegativeIndex(t *testing.T) {
	doc := map[string]any{"x": []any{1, 2}}
	got, ok := Resolve(doc, "/x/-1")
	require.False(t, ok)
	require.Nil(t, got)
}
