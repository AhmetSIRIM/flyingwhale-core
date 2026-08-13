package daykey_test

import (
	"testing"
	"time"

	"github.com/AhmetSIRIM/flyingwhale-core/daykey"
)

// TestKeyDerivesTheLocalCalendarDay covers the core rule: the day key is
// the Gregorian date of the instant obtained by shifting the UTC clock by
// the caller's offset in minutes.
func TestKeyDerivesTheLocalCalendarDay(t *testing.T) {
	istanbul := time.FixedZone("+03", 3*60*60)

	tests := []struct {
		name            string
		now             time.Time
		tzOffsetMinutes int
		want            string
	}{
		{
			name:            "utc noon with no offset",
			now:             time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC),
			tzOffsetMinutes: 0,
			want:            "2026-07-29",
		},
		{
			name:            "positive offset rolls into the next local day",
			now:             time.Date(2026, time.July, 29, 22, 30, 0, 0, time.UTC),
			tzOffsetMinutes: 180,
			want:            "2026-07-30",
		},
		{
			name:            "negative offset stays on the previous local day",
			now:             time.Date(2026, time.July, 29, 2, 0, 0, 0, time.UTC),
			tzOffsetMinutes: -300,
			want:            "2026-07-28",
		},
		{
			name:            "negative half hour offset crosses midnight backwards",
			now:             time.Date(2026, time.July, 29, 0, 15, 0, 0, time.UTC),
			tzOffsetMinutes: -30,
			want:            "2026-07-28",
		},
		{
			name:            "one second before local midnight",
			now:             time.Date(2026, time.July, 29, 23, 59, 59, 0, time.UTC),
			tzOffsetMinutes: 0,
			want:            "2026-07-29",
		},
		{
			name:            "exactly at local midnight",
			now:             time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC),
			tzOffsetMinutes: 0,
			want:            "2026-07-30",
		},
		{
			name:            "a non utc now is normalized before shifting",
			now:             time.Date(2026, time.July, 29, 2, 0, 0, 0, istanbul),
			tzOffsetMinutes: 0,
			want:            "2026-07-28",
		},
		{
			name:            "year boundary crossing",
			now:             time.Date(2026, time.December, 31, 23, 30, 0, 0, time.UTC),
			tzOffsetMinutes: 60,
			want:            "2027-01-01",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := daykey.Key(tc.now, tc.tzOffsetMinutes)
			if got != tc.want {
				t.Errorf("Key(%s, %d) = %q, want %q", tc.now.Format(time.RFC3339), tc.tzOffsetMinutes, got, tc.want)
			}
		})
	}
}

// TestKeyClampsTimezoneOffset locks the accepted offset range to [-720, 840]
// minutes (UTC-12 to UTC+14). Every case picks an instant where clamping
// and not clamping land on different calendar days, so an unclamped
// implementation cannot pass by accident.
func TestKeyClampsTimezoneOffset(t *testing.T) {
	tests := []struct {
		name            string
		now             time.Time
		tzOffsetMinutes int
		want            string
	}{
		{
			name:            "offset above the maximum clamps to utc plus fourteen",
			now:             time.Date(2026, time.July, 29, 9, 59, 0, 0, time.UTC),
			tzOffsetMinutes: 1000,
			want:            "2026-07-29",
		},
		{
			name:            "offset below the minimum clamps to utc minus twelve",
			now:             time.Date(2026, time.July, 29, 12, 1, 0, 0, time.UTC),
			tzOffsetMinutes: -900,
			want:            "2026-07-29",
		},
		{
			name:            "the maximum offset itself is not clamped away",
			now:             time.Date(2026, time.July, 29, 10, 1, 0, 0, time.UTC),
			tzOffsetMinutes: 840,
			want:            "2026-07-30",
		},
		{
			name:            "the minimum offset itself is not clamped away",
			now:             time.Date(2026, time.July, 29, 11, 59, 0, 0, time.UTC),
			tzOffsetMinutes: -720,
			want:            "2026-07-28",
		},
		{
			name:            "an absurd positive offset does not skip days",
			now:             time.Date(2026, time.July, 29, 9, 59, 0, 0, time.UTC),
			tzOffsetMinutes: 100000,
			want:            "2026-07-29",
		},
		{
			name:            "an absurd negative offset does not skip days",
			now:             time.Date(2026, time.July, 29, 12, 1, 0, 0, time.UTC),
			tzOffsetMinutes: -100000,
			want:            "2026-07-29",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := daykey.Key(tc.now, tc.tzOffsetMinutes)
			if got != tc.want {
				t.Errorf("Key(%s, %d) = %q, want %q", tc.now.Format(time.RFC3339), tc.tzOffsetMinutes, got, tc.want)
			}
		})
	}
}
