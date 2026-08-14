// Package daykey derives a calendar day key from an instant and a caller
// supplied timezone offset.
package daykey

import "time"

// A caller supplied timezone offset is client asserted, so it is clamped to
// the real range of UTC offsets before it can move the calendar day.
const (
	MinTZOffsetMinutes = -720
	MaxTZOffsetMinutes = 840
)

const dayKeyLayout = "2006-01-02"

// Key returns the calendar day, formatted as "2006-01-02", of the instant
// obtained by shifting now by tzOffsetMinutes. tzOffsetMinutes is clamped to
// [MinTZOffsetMinutes, MaxTZOffsetMinutes] before it is applied.
func Key(now time.Time, tzOffsetMinutes int) string {
	clamped := min(max(tzOffsetMinutes, MinTZOffsetMinutes), MaxTZOffsetMinutes)
	return now.UTC().Add(time.Duration(clamped) * time.Minute).Format(dayKeyLayout)
}
