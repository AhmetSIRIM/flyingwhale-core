package httpx

import (
	"slices"
	"sort"
	"strconv"
	"strings"
)

// languageRange is one entry of a parsed Accept-Language header: a language
// tag together with its relative quality value, defaulting to 1 when the
// header omits "q".
type languageRange struct {
	tag string
	q   float64
}

// NegotiateLanguage picks the best of the supported languages for an
// Accept-Language header, falling back to fallback when the header is
// absent, unparseable, or names nothing in supported.
//
// A bare "*" is treated as a match for fallback rather than for some
// arbitrary supported language: "*" means the client has no real preference,
// and fallback is the caller's answer to "no preference", so this keeps
// both cases consistent instead of picking one supported language over the
// others for no principled reason.
func NegotiateLanguage(header string, supported []string, fallback string) string {
	for _, candidate := range parseAcceptLanguage(header) {
		if candidate.tag == "*" {
			return fallback
		}
		if primary := primarySubtag(candidate.tag); slices.Contains(supported, primary) {
			return primary
		}
	}
	return fallback
}

// parseAcceptLanguage turns a raw Accept-Language header value into its
// language ranges, ordered from most to least preferred.
//
// An entry with q=0 is dropped per RFC 7231 (it is an explicit refusal, not
// a low preference). An entry whose tag or quality syntax does not parse is
// dropped too, rather than failing the whole header: one bad entry in an
// otherwise valid list should not discard the good ones. Ties in quality
// keep the header's own order, since that is the only tie-break the client
// itself expressed.
func parseAcceptLanguage(header string) []languageRange {
	var ranges []languageRange
	for _, entry := range strings.Split(header, ",") {
		if candidate, ok := parseLanguageRange(entry); ok && candidate.q > 0 {
			ranges = append(ranges, candidate)
		}
	}
	sort.SliceStable(ranges, func(i, j int) bool { return ranges[i].q > ranges[j].q })
	return ranges
}

// parseLanguageRange parses one comma-separated entry, e.g. " tr ; q=0.9 ".
func parseLanguageRange(entry string) (languageRange, bool) {
	fields := strings.Split(entry, ";")
	tag := strings.ToLower(strings.TrimSpace(fields[0]))
	if !isValidLanguageTag(tag) {
		return languageRange{}, false
	}

	q := 1.0
	for _, param := range fields[1:] {
		name, value, hasValue := strings.Cut(param, "=")
		if !hasValue || strings.ToLower(strings.TrimSpace(name)) != "q" {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || parsed < 0 || parsed > 1 {
			return languageRange{}, false
		}
		q = parsed
	}
	return languageRange{tag: tag, q: q}, true
}

// isValidLanguageTag accepts "*" and BCP 47 shaped tags: a primary subtag
// followed by zero or more "-" separated subtags, each 1-8 alphanumeric
// characters. This is looser than the real BCP 47 grammar (which restricts
// where digits may appear), because the only thing NegotiateLanguage does
// with a tag is compare its primary subtag against a caller-supplied list of
// known codes, so a stricter grammar would add complexity without changing
// any outcome.
func isValidLanguageTag(tag string) bool {
	if tag == "*" {
		return true
	}
	if tag == "" {
		return false
	}
	for _, subtag := range strings.Split(tag, "-") {
		if len(subtag) == 0 || len(subtag) > 8 || !isAlphanumeric(subtag) {
			return false
		}
	}
	return true
}

func isAlphanumeric(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// primarySubtag returns the first "-" separated part of a language tag,
// which is what a region or script subtag like "pt-BR" or "zh-Hans" is
// matched on.
func primarySubtag(tag string) string {
	primary, _, _ := strings.Cut(tag, "-")
	return primary
}
