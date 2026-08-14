package httpx

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestCodeWireValues pins the wire strings of the two codes this package
// owns: a client maps these values against known cases, so a rename here is
// a breaking change.
func TestCodeWireValues(t *testing.T) {
	tests := []struct {
		name string
		code Code
		want string
	}{
		{name: "internal", code: CodeInternal, want: "internal"},
		{name: "too many requests", code: CodeTooManyRequests, want: "too_many_requests"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if diff := cmp.Diff(testCase.want, string(testCase.code)); diff != "" {
				t.Errorf("unexpected wire value (-want +got):\n%s", diff)
			}
		})
	}
}
