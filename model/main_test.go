package model

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

func TestV1NotificationRequestTypeValidation(t *testing.T) {
	tests := []struct {
		name             string
		notificationType string
		wantValid        bool
	}{
		{name: "a simple type is accepted", notificationType: "analysis", wantValid: true},
		{name: "underscores are accepted", notificationType: "data_transfer", wantValid: true},
		{name: "a type of the maximum length is accepted", notificationType: strings.Repeat("a", 32), wantValid: true},
		{name: "a missing type is rejected", notificationType: "", wantValid: false},
		{name: "a type that is too long for its column is rejected", notificationType: strings.Repeat("a", 33), wantValid: false},
		{name: "a period is rejected because it separates routing key components",
			notificationType: "data.transfer", wantValid: false},
	}

	v := validator.New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &V1NotificationRequest{
				Type:    tt.notificationType,
				User:    "sarahr",
				Subject: "some job status changed",
			}

			err := v.Struct(request)
			if tt.wantValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
