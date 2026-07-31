// Package name defines the semantic identity of Mace identifiers.
package name

import "golang.org/x/text/unicode/norm"

// Name is the canonical semantic spelling of a Mace identifier.
type Name string

// NormalizeName converts an identifier spelling to Unicode NFC. It deliberately
// does not perform compatibility, case, locale, or confusable-character mapping.
func NormalizeName(raw string) Name {
	if norm.NFC.IsNormalString(raw) {
		return Name(raw)
	}
	return Name(norm.NFC.String(raw))
}
