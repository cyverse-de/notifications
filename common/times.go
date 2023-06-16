package common

import (
	"fmt"
	"regexp"
	"time"

	"github.com/pkg/errors"
)

// FormatTimestamp converts an instance of time.Time to a string representation
// of the number of milliseconds since the epoch.
func FormatTimestamp(timestamp time.Time) string {
	return fmt.Sprintf("%d", int(timestamp.UnixNano()/1000000))
}

// FixTimestamp converts a string representation of a timestamp to milliseconds
// since the epoch if it's not in that format already.
func FixTimestamp(original string) (string, error) {
	wrapMsg := "unable to parse timestamp"

	// If the original is empty, simply return it.
	if original == "" {
		return original, nil
	}

	// If the original already appears to be an epoch time, simply return it.
	epochRegexp := regexp.MustCompile(`^\d+$`)
	if epochRegexp.MatchString(original) {
		return original, nil
	}

	// Attempt to parse the timestamp. We only support one format for now.
	timestamp, err := time.Parse(time.RFC3339Nano, original)
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	// Return the timestamp as nanoseconds since the epoch.
	return FormatTimestamp(timestamp), nil
}
