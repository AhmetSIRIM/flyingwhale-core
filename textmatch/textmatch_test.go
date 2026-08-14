package textmatch_test

import (
	"testing"

	"github.com/AhmetSIRIM/flyingwhale-core/textmatch"
)

// TestMatchesKeyword pins the combined contract: a keyword with a space or
// a CJK rune matches by plain containment, every other keyword must land on
// a word boundary. Expected values are taken from running the ported
// algorithm's source behavior directly, not guessed.
func TestMatchesKeyword(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		keyword string
		want    bool
	}{
		{
			name:    "cjk keyword matches as a substring anywhere",
			text:    "咖啡が飲みたい",
			keyword: "咖啡",
			want:    true,
		},
		{
			name:    "cjk keyword matches inside a run of the same script",
			text:    "咖啡咖啡",
			keyword: "咖啡",
			want:    true,
		},
		{
			name:    "cjk keyword matches surrounded by non-cjk runes",
			text:    "x咖啡x",
			keyword: "咖啡",
			want:    true,
		},
		{
			name:    "latin keyword inside a longer word does not match",
			text:    "barbershop",
			keyword: "bar",
			want:    false,
		},
		{
			name:    "latin keyword matches the whole text",
			text:    "bar",
			keyword: "bar",
			want:    true,
		},
		{
			name:    "latin keyword matches at the start against trailing punctuation",
			text:    "bar!",
			keyword: "bar",
			want:    true,
		},
		{
			name:    "latin keyword matches at the end against leading punctuation",
			text:    "!bar",
			keyword: "bar",
			want:    true,
		},
		{
			name:    "latin keyword matches in the middle between punctuation on both sides",
			text:    "a bar!!b",
			keyword: "bar",
			want:    true,
		},
		{
			name:    "empty text never matches a non-empty keyword",
			text:    "",
			keyword: "bar",
			want:    false,
		},
		{
			name:    "an empty keyword matches nothing",
			text:    "bar",
			keyword: "",
			want:    false,
		},
		{
			name:    "keyword longer than the text cannot match",
			text:    "bar",
			keyword: "barlonger",
			want:    false,
		},
		{
			name:    "multi-byte accented keyword matches the whole text",
			text:    "café",
			keyword: "café",
			want:    true,
		},
		{
			name:    "multi-byte accented keyword inside a longer word does not match",
			text:    "xcafé",
			keyword: "café",
			want:    false,
		},
		{
			name:    "multi-byte accented keyword followed by more letters does not match",
			text:    "caféx",
			keyword: "café",
			want:    false,
		},
		{
			name:    "a trailing digit blocks the boundary like a letter would",
			text:    "bar1",
			keyword: "bar",
			want:    false,
		},
		{
			name:    "a leading digit blocks the boundary like a letter would",
			text:    "1bar",
			keyword: "bar",
			want:    false,
		},
		{
			name:    "digits on both sides block the boundary",
			text:    "3bar5",
			keyword: "bar",
			want:    false,
		},
		{
			name:    "digits separated by spaces do not block the boundary",
			text:    "3 bar 5",
			keyword: "bar",
			want:    true,
		},
		{
			name:    "a keyword containing a space ignores word boundaries and does not match mid-word",
			text:    "streetxfoody",
			keyword: "street food",
			want:    false,
		},
		{
			name:    "a keyword containing a space matches as a plain substring",
			text:    "xstreet foody",
			keyword: "street food",
			want:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := textmatch.MatchesKeyword(test.text, test.keyword)
			if got != test.want {
				t.Errorf("MatchesKeyword(%q, %q) = %v, want %v", test.text, test.keyword, got, test.want)
			}
		})
	}
}

// TestContainsCJK pins which scripts route MatchesKeyword to the
// containment path: Han, Hiragana, and Katakana. Everything else, including
// accented Latin letters, takes the word-boundary path instead.
func TestContainsCJK(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty string has no cjk rune", value: "", want: false},
		{name: "plain ascii has no cjk rune", value: "coffee", want: false},
		{name: "accented latin has no cjk rune", value: "café", want: false},
		{name: "han rune is cjk", value: "咖啡", want: true},
		{name: "hiragana rune is cjk", value: "たい", want: true},
		{name: "katakana rune is cjk", value: "コーヒー", want: true},
		{name: "a single cjk rune among ascii still counts", value: "x咖x", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := textmatch.ContainsCJK(test.value)
			if got != test.want {
				t.Errorf("ContainsCJK(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

// TestContainsAtWordBoundary exercises the boundary scan in isolation from
// the containment fallback that MatchesKeyword layers on top of it.
func TestContainsAtWordBoundary(t *testing.T) {
	tests := []struct {
		name     string
		haystack string
		keyword  string
		want     bool
	}{
		{name: "empty haystack cannot contain a non-empty keyword", haystack: "", keyword: "bar", want: false},
		{name: "keyword longer than the haystack cannot match", haystack: "bar", keyword: "barlonger", want: false},
		{name: "an empty keyword matches nothing against an empty haystack", haystack: "", keyword: "", want: false},
		{name: "an empty keyword matches nothing against a haystack ending in a word rune", haystack: "bar", keyword: "", want: false},
		{name: "an empty keyword matches nothing against a haystack ending in a non-word rune", haystack: "hi!", keyword: "", want: false},
		{name: "match inside a longer word is rejected", haystack: "barbershop", keyword: "bar", want: false},
		{name: "match spanning the whole haystack is accepted", haystack: "bar", keyword: "bar", want: true},
		{name: "a rejected occurrence inside a word is followed by an accepted one", haystack: "xbar bar", keyword: "bar", want: true},
		{name: "a digit immediately after the match blocks the boundary", haystack: "bar1", keyword: "bar", want: false},
		{name: "a digit immediately before the match blocks the boundary", haystack: "1bar", keyword: "bar", want: false},
		{name: "a space on both sides of a digit-guarded match is accepted", haystack: "3 bar 5", keyword: "bar", want: true},
		{name: "an accented multi-byte keyword matches the whole haystack", haystack: "café", keyword: "café", want: true},
		{name: "an accented multi-byte keyword followed by a letter is rejected", haystack: "caféx", keyword: "café", want: false},
		{name: "a cjk rune before the match blocks the boundary since it is a letter", haystack: "咖啡bar", keyword: "bar", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := textmatch.ContainsAtWordBoundary(test.haystack, test.keyword)
			if got != test.want {
				t.Errorf("ContainsAtWordBoundary(%q, %q) = %v, want %v", test.haystack, test.keyword, got, test.want)
			}
		})
	}
}
