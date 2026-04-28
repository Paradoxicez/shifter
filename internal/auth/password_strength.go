package auth

import "unicode"

// Strength is the UI-facing tier returned by PasswordStrength.
// Iota order is part of the public contract: WeakStrength=0 is the
// JSON/numeric default, and downstream code may compare with `>= StrengthGood`.
type Strength int

const (
	StrengthWeak Strength = iota
	StrengthOK
	StrengthGood
	StrengthStrong
)

// String returns the lowercase tier name used by the UI strength meter
// (UI-SPEC §"Force-change-password screen", line 234).
func (s Strength) String() string {
	switch s {
	case StrengthWeak:
		return "weak"
	case StrengthOK:
		return "ok"
	case StrengthGood:
		return "good"
	case StrengthStrong:
		return "strong"
	}
	return "unknown"
}

// PasswordStrength returns a UI tier for the given password string.
//
// Heuristic (UI-SPEC line 548 hint: "At least 12 characters with mixed case,
// a number, and a symbol."):
//
//	length < 12               → weak
//	length 12+, 1–2 classes   → ok
//	length 12+, 3 classes     → good   (length 16+ promotes to strong)
//	length 12+, 4 classes     → good   (length 14+ promotes to strong)
//
// Character classes counted: uppercase, lowercase, digit, symbol/punct.
// Stateless, no I/O, no allocations beyond per-rune classification.
func PasswordStrength(password string) Strength {
	if len(password) < 12 {
		return StrengthWeak
	}
	var hasUpper, hasLower, hasDigit, hasSymbol bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r), unicode.IsSymbol(r):
			hasSymbol = true
		}
	}
	classes := 0
	for _, b := range []bool{hasUpper, hasLower, hasDigit, hasSymbol} {
		if b {
			classes++
		}
	}
	switch {
	case classes == 4 && len(password) >= 14:
		return StrengthStrong
	case classes >= 3 && len(password) >= 16:
		return StrengthStrong
	case classes >= 3:
		return StrengthGood
	case classes == 4:
		return StrengthGood
	default:
		return StrengthOK
	}
}
