package query

import (
	"fmt"
	"strings"
)

// V1ListingSortField represents an acceptable value for a notification listing in version 1 of the API.
type V1ListingSortField string

// Enumeration constants.
const (
	V1ListingSortFieldDateCreated V1ListingSortField = "date_created"
	V1ListingSortFieldTimestamp   V1ListingSortField = "timestamp"
	V1ListingSortFieldUUID        V1ListingSortField = "uuid"
	V1ListingSortFieldSubject     V1ListingSortField = "subject"
)

// V1ListingSortFieldParam represents a query parameter used to specify the sort field in a notification listing for
// version 1 of the API.
type V1ListingSortFieldParam struct {
	Value        *V1ListingSortField
	DefaultValue *V1ListingSortField
}

// NewV1ListingSortFieldParam returns a new sort field parameter.
func NewV1ListingSortFieldParam(defaultValue *V1ListingSortField) *V1ListingSortFieldParam {
	return &V1ListingSortFieldParam{
		Value:        nil,
		DefaultValue: defaultValue,
	}
}

// FromString sets the parameter value based on the value of a string.
func (p *V1ListingSortFieldParam) FromString(s string) error {
	sortField := V1ListingSortField(strings.ToLower(s))
	switch sortField {
	case V1ListingSortFieldDateCreated, V1ListingSortFieldTimestamp, V1ListingSortFieldUUID, V1ListingSortFieldSubject:
		p.Value = &sortField
		return nil
	}
	return fmt.Errorf("invalid sort field: %s", s)
}

// String returns the string representation of a sort field in a notification listing for version 1 of the API.
func (p *V1ListingSortFieldParam) String() string {
	v := p.GetValue().(*V1ListingSortField)
	if v != nil {
		return string(*v)
	}
	return ""
}

// GetValue returns the value of a sort field parameter.
func (p *V1ListingSortFieldParam) GetValue() interface{} {
	if p.Value != nil {
		return p.Value
	}
	return p.DefaultValue
}

// GetDefaultValue returns the default value of a sort field parameter.
func (p *V1ListingSortFieldParam) GetDefaultValue() interface{} {
	return p.DefaultValue
}
