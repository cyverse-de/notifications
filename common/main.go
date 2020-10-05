package common

import (
	"regexp"

	"github.com/mcnijman/go-emailaddress"
)

// AMQPSettings represents the settings that we require in order to connect to the AMQP exchange.
type AMQPSettings struct {
	URI          string
	ExchangeName string
	ExchangeType string
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
