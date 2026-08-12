package common

import (
	"fmt"
	"regexp"
	"time"

	"github.com/mcnijman/go-emailaddress"
	"github.com/pkg/errors"
)

// AMQPSettings represents the settings that we require in order to connect to the AMQP exchange.
type AMQPSettings struct {
	URI          string
	ExchangeName string
	ExchangeType string
}

// Notification represents a single notification to be recorded in the database.
type Notification struct {
	ID               string
	NotificationType string
	User             string
	Subject          string
	Seen             bool
	Deleted          bool
	TimeCreated      time.Time
	Message          string
	RoutingKey       string
}

// FixTimestampInMap converts the timestamp stored under a key in a map to milliseconds since the
// epoch, leaving the map untouched if the key is absent.
func FixTimestampInMap(m map[string]any, k string) error {
	wrapMsg := fmt.Sprintf("unable to fix the timestamp in key '%s'", k)

	v, present := m[k]
	if !present {
		return nil
	}

	// Only the types that the json package can produce need to be handled here.
	var stringValue string
	switch val := v.(type) {
	case string:
		stringValue = val
	case float64:
		stringValue = fmt.Sprintf("%d", int64(val))
	default:
		return fmt.Errorf("%s: %s", wrapMsg, "invalid data type")
	}

	convertedValue, err := FixTimestamp(stringValue)
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	m[k] = convertedValue

	return nil
}

// ValidateEmailAddress returns an error if the format of an email address is invalid.
func ValidateEmailAddress(emailAddress string) error {
	_, err := emailaddress.Parse(emailAddress)
	return err
}

// IsBlank returns true if a string is blank.
func IsBlank(s string) bool {
	blank, err := regexp.MatchString("^\\s*$", s)
	if err != nil {
		panic(err)
	}
	return blank
}
