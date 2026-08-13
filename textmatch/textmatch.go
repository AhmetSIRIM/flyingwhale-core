// Package textmatch matches a keyword against a piece of text across
// scripts that delimit words differently. Latin, Cyrillic, and other
// alphabetic scripts must match at a word boundary; scripts that write
// without inter-word separators, such as Han, Hiragana, and Katakana, match
// by plain containment instead. The package performs no case folding: a
// caller normalizes casing, with whatever locale awareness it needs, before
// calling into it.
package textmatch

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MatchesKeyword reports whether keyword occurs in text under the rule
// appropriate for its script. A keyword that contains a space, or that
// contains a Han, Hiragana, or Katakana rune, matches anywhere it occurs as
// a substring: multi-word phrases carry their own internal spacing, and CJK
// scripts have no word-delimiting whitespace or punctuation for a boundary
// scan to anchor on. Every other keyword must occur at a word boundary, as
// determined by ContainsAtWordBoundary. An empty keyword matches nothing.
func MatchesKeyword(text, keyword string) bool {
	if strings.Contains(keyword, " ") || ContainsCJK(keyword) {
		return strings.Contains(text, keyword)
	}
	return ContainsAtWordBoundary(text, keyword)
}

// ContainsCJK reports whether value contains a Han, Hiragana, or Katakana
// rune. Words in these scripts are not set off by whitespace or
// punctuation, so MatchesKeyword treats a keyword written in one of them as
// matching by plain containment rather than by a word-boundary scan.
func ContainsCJK(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			return true
		}
	}
	return false
}

// ContainsAtWordBoundary reports whether keyword occurs in haystack. An
// occurrence counts only if it is delimited on both sides, by a non-word
// rune or by the edge of the string, rather than sitting in the middle of a
// longer word. It walks successive occurrences of keyword from left to
// right, checking the boundary condition at each, and reports a match on
// the first occurrence that satisfies it. An empty keyword matches nothing:
// there is no occurrence for a boundary to anchor on, so it is rejected
// outright instead of being scanned for.
func ContainsAtWordBoundary(haystack, keyword string) bool {
	if keyword == "" {
		return false
	}
	for offset := 0; offset+len(keyword) <= len(haystack); {
		index := strings.Index(haystack[offset:], keyword)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(keyword)
		runeBefore, _ := utf8.DecodeLastRuneInString(haystack[:start])
		runeAfter, _ := utf8.DecodeRuneInString(haystack[end:])
		if !isWordRune(runeBefore) && !isWordRune(runeAfter) {
			return true
		}
		_, width := utf8.DecodeRuneInString(haystack[start:])
		offset = start + width
	}
	return false
}

// isWordRune reports whether r counts as part of a word for a boundary
// check: a letter or a number. Number is Unicode's general numeric
// category, wider than the ASCII decimal digits alone, so non-decimal
// numerals also block a boundary match the same way decimal digits do.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}
