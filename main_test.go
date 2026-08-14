package main

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// completeConfig returns the settings for a fully configured service, which each test case then
// removes or overrides settings from.
func completeConfig() map[string]string {
	return map[string]string{
		"amqp.uri":                 "amqp://guest:guest@rabbit:5672/de",
		"amqp.exchange.name":       "de",
		"amqp.exchange.type":       "topic",
		"notifications.db.uri":     "postgresql://guest:guest@dedb:5432/notifications?sslmode=disable",
		"notifications.uid.domain": "iplantcollaborative.org",
		"email.request":            "support@example.org",
		"email.fromAddress":        "noreply@example.org",
		"email.smtpHost":           "local-exim",
		"de.base":                  "https://de.example.org",
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		omit      []string
		overrides map[string]string
		missing   []string
	}{
		{
			name: "complete configuration",
		},
		{
			name:    "missing support email address",
			omit:    []string{"email.request"},
			missing: []string{"email.request"},
		},
		{
			name:    "missing database URI",
			omit:    []string{"notifications.db.uri"},
			missing: []string{"notifications.db.uri"},
		},
		{
			// Without it every user lookup would miss: callers send bare usernames and the
			// DE stores qualified ones.
			name:    "missing user domain",
			omit:    []string{"notifications.uid.domain"},
			missing: []string{"notifications.uid.domain"},
		},
		{
			name:    "missing exchange settings",
			omit:    []string{"amqp.exchange.name", "amqp.exchange.type"},
			missing: []string{"amqp.exchange.name", "amqp.exchange.type"},
		},
		{
			// A setting that's present but blank is just as unusable as one that's absent.
			name:      "blank support email address",
			overrides: map[string]string{"email.request": "   "},
			missing:   []string{"email.request"},
		},
		{
			// Absorbed from de-mailer: without these the service starts and then fails
			// every send, rather than refusing to start.
			name:    "missing email delivery settings",
			omit:    []string{"email.fromAddress", "email.smtpHost", "de.base"},
			missing: []string{"email.fromAddress", "email.smtpHost", "de.base"},
		},
		{
			name:    "empty configuration",
			omit:    requiredConfigKeys,
			missing: requiredConfigKeys,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			settings := completeConfig()
			for _, key := range tt.omit {
				delete(settings, key)
			}
			for key, value := range tt.overrides {
				settings[key] = value
			}

			cfg := viper.New()
			for key, value := range settings {
				cfg.Set(key, value)
			}

			err := validateConfig(cfg)
			if len(tt.missing) == 0 {
				assert.NoError(err)
				return
			}

			assert.Error(err)
			for _, key := range tt.missing {
				assert.Contains(err.Error(), key, "the error should name the missing setting")
			}
		})
	}
}
