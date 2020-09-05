package query

import "time"

// TimestampParam represents a query parameter used to specify a timestamp.
type TimestampParam struct {
	Value        *time.Time
	DefaultValue *time.Time
}

// NewTimestampParam returns a new timestmp parameter.
func NewTimestampParam(defaultValue *time.Time) *TimestampParam {
	return &TimestampParam{
		Value:        nil,
		DefaultValue: defaultValue,
	}
}

// FromString sets the parameter value based on the value of a string.
func (p *TimestampParam) FromString(s string) error {
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	p.Value = &v
	return nil
}

// String returns the string representation of a timestamp parameter.
func (p *TimestampParam) String() string {
	v := p.GetValue().(*time.Time)
	if v == nil {
		return ""
	}
	return v.Format(time.RFC3339Nano)
}

// GetValue returns the value of a timestamp parameter.
func (p *TimestampParam) GetValue() interface{} {
	if p.Value != nil {
		return p.Value
	}
	return p.DefaultValue
}

// GetDefaultValue returns the default value of a timestamp parameter.
func (p *TimestampParam) GetDefaultValue() interface{} {
	return p.DefaultValue
}
