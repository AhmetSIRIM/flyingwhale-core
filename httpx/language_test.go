package httpx

import "testing"

var testSupportedLanguages = []string{"ar", "de", "en", "es", "fr", "ja", "tr", "zh"}

// TestNegotiateLanguage covers the Accept-Language parsing and matching rules:
// quality values, whitespace, case, subtags, the "*" wildcard, q=0 exclusion,
// and fallback on no match, no header, or a malformed header.
func TestNegotiateLanguage(t *testing.T) {
	testCases := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "realistic browser header picks the top quality supported tag",
			header: "tr-TR,tr;q=0.9,en-US;q=0.8,en;q=0.7",
			want:   "tr",
		},
		{
			name:   "header matches nothing supported falls back to english",
			header: "fi-FI,fi;q=0.9,it;q=0.8",
			want:   "en",
		},
		{
			name:   "absent header falls back to english",
			header: "",
			want:   "en",
		},
		{
			name:   "bare wildcard falls back to english",
			header: "*",
			want:   "en",
		},
		{
			name:   "wildcard outranking a supported tag yields the fallback",
			header: "de;q=0.5,*;q=0.9",
			want:   "en",
		},
		{
			name:   "q=0 on the top choice excludes it in favor of the next",
			header: "tr;q=0,de;q=0.5",
			want:   "de",
		},
		{
			name:   "malformed header falls back to english",
			header: "tr_TR",
			want:   "en",
		},
		{
			name:   "whitespace around tags and quality values is tolerated",
			header: " tr ; q=0.9 , en ; q=0.8 ",
			want:   "tr",
		},
		{
			name:   "tags compare case-insensitively",
			header: "TR-tr;q=0.9",
			want:   "tr",
		},
		{
			name:   "a region subtag matches its supported primary subtag",
			header: "zh-Hans;q=0.9",
			want:   "zh",
		},
		{
			name:   "an unsupported primary subtag with a supported region-like suffix still falls back",
			header: "pt-BR;q=0.9",
			want:   "en",
		},
		{
			name:   "lower quality supported tag still wins over an unsupported higher quality one",
			header: "fi;q=0.9,es;q=0.5",
			want:   "es",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := NegotiateLanguage(testCase.header, testSupportedLanguages, "en")
			if got != testCase.want {
				t.Errorf("NegotiateLanguage(%q) = %q, want %q", testCase.header, got, testCase.want)
			}
		})
	}
}

// TestParseAcceptLanguageOrdersByQualityThenHeaderOrder pins the tie-break
// rule directly: entries are ranked by descending quality, and entries that
// share a quality keep the order the client listed them in.
func TestParseAcceptLanguageOrdersByQualityThenHeaderOrder(t *testing.T) {
	ranges := parseAcceptLanguage("fr;q=0.8,de;q=0.8,tr;q=0.9,ja;q=0")

	var tags []string
	for _, r := range ranges {
		tags = append(tags, r.tag)
	}

	want := []string{"tr", "fr", "de"}
	if len(tags) != len(want) {
		t.Fatalf("parseAcceptLanguage tags = %v, want %v", tags, want)
	}
	for i, tag := range want {
		if tags[i] != tag {
			t.Errorf("tags[%d] = %q, want %q (full: %v)", i, tags[i], tag, tags)
		}
	}
}
