package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixTimestampInMap(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		key     string
		want    any
		wantErr bool
	}{
		{
			name:  "an absent key is left alone",
			input: map[string]any{},
			key:   "startdate",
			want:  nil,
		},
		{
			name:  "an RFC3339 string becomes milliseconds",
			input: map[string]any{"startdate": "2020-07-07T17:59:59-07:00"},
			key:   "startdate",
			want:  "1594169999000",
		},
		{
			name:  "a numeric value is stringified then converted",
			input: map[string]any{"startdate": float64(1594169999000)},
			key:   "startdate",
			want:  "1594169999000",
		},
		{
			name:    "an unsupported type is an error",
			input:   map[string]any{"startdate": []string{"nope"}},
			key:     "startdate",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FixTimestampInMap(tt.input, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, tt.input[tt.key])
		})
	}
}
