package webhook_test

import (
	"testing"

	"github.com/AhmetSIRIM/flyingwhale-core/webhook"
)

// TestAuthorized covers the shapes an Authorization header can take against
// a configured shared secret: an absent secret, a raw match, a bearer
// prefixed match, a mismatch, and near misses that must not be treated as
// matches.
func TestAuthorized(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		header string
		want   bool
	}{
		{
			name:   "an empty secret denies an empty header",
			secret: "",
			header: "",
			want:   false,
		},
		{
			name:   "an empty secret denies any header",
			secret: "",
			header: "some-value",
			want:   false,
		},
		{
			name:   "a header equal to the raw secret is allowed",
			secret: "topsecret",
			header: "topsecret",
			want:   true,
		},
		{
			name:   "a header carrying the bearer prefix is allowed",
			secret: "topsecret",
			header: "Bearer topsecret",
			want:   true,
		},
		{
			name:   "a wrong secret is denied",
			secret: "topsecret",
			header: "othersecret",
			want:   false,
		},
		{
			name:   "a bare bearer prefix is denied against a non empty secret",
			secret: "topsecret",
			header: "Bearer ",
			want:   false,
		},
		{
			name:   "trailing garbage appended to the secret is denied",
			secret: "topsecret",
			header: "topsecretXXX",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := webhook.Authorized(tt.secret, tt.header)
			if got != tt.want {
				t.Errorf("Authorized(%q, %q) = %t, want %t", tt.secret, tt.header, got, tt.want)
			}
		})
	}
}
