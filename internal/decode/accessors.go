// Package decode turns the inner JSON payload of a GetShoppingResults response
// into structured flight data. It is I/O-free and operates on values already
// produced by [encoding/json] (so: []any, float64, string, bool, nil), which
// makes it exhaustively testable against captured fixtures.
//
// The position layout is undocumented and reverse-engineered; every index is a
// named constant in flight.go and cross-referenced to docs/wire/shopping-results.md.
// Individual rows that do not match the expected shape are skipped rather than
// failing the whole decode — Google routinely mixes in half-populated sponsor
// rows.
package decode

// at returns v[i] when v is an []any with i in range, else nil.
func at(v any, i int) any {
	s, ok := v.([]any)
	if !ok || i < 0 || i >= len(s) {
		return nil
	}
	return s[i]
}

// path walks a chain of indices, stopping at the first miss.
func path(v any, idx ...int) any {
	for _, i := range idx {
		v = at(v, i)
	}
	return v
}

// asStr returns a non-empty string, else "".
func asStr(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// asInt returns an integer value; JSON numbers arrive as float64. bool is not
// an int. ok is false when v is not numeric.
func asInt(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// asNonNegInt returns v as an int ≥ 0, else (0, false).
func asNonNegInt(v any) (int, bool) {
	n, ok := asInt(v)
	if !ok || n < 0 {
		return 0, false
	}
	return n, true
}

// asFloat returns v as a float64. bool is rejected.
func asFloat(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// asBool returns a *bool: nil when v is not a JSON bool, preserving the
// "unknown" state Google encodes as null.
func asBool(v any) *bool {
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
}

// isSlice reports whether v is a JSON array.
func isSlice(v any) bool {
	_, ok := v.([]any)
	return ok
}

// sliceOf returns v as []any, or nil.
func sliceOf(v any) []any {
	s, _ := v.([]any)
	return s
}
