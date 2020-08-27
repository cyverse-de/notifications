package query

import (
	"fmt"
	"strings"
)

// SortOrder represents an acceptable value for a sort order.
type SortOrder string

// Enumeration constants.
const (
	SortOrderAscending  SortOrder = "ASC"
	SortOrderDescending SortOrder = "DESC"
)

// SortOrderParam represents a query parameter used to specify sort order.
type SortOrderParam struct {
	Value        *SortOrder
	DefaultValue *SortOrder
}

// NewSortOrderParam returns a new sort order parameter.
func NewSortOrderParam(defaultValue *SortOrder) *SortOrderParam {
	return &SortOrderParam{
		Value:        nil,
		DefaultValue: defaultValue,
	}
}

// FromString sets a sort order parameter value based on the value of a string.
func (p *SortOrderParam) FromString(s string) error {
	sortOrder := SortOrder(strings.ToUpper(s))
	switch sortOrder {
	case SortOrderAscending, SortOrderDescending:
		p.Value = &sortOrder
		return nil
	}
	return fmt.Errorf("invalid sort order: %s", s)
}

// String returns a string representation of a sort order.
func (p *SortOrderParam) String() string {
	v := p.GetValue().(*SortOrder)
	if v != nil {
		return string(*v)
	}
	return ""
}

// GetValue returns the value of the sort order parameter.
func (p *SortOrderParam) GetValue() interface{} {
	if p.Value != nil {
		return p.Value
	}
	return p.DefaultValue
}

// GetDefaultValue returns the default value of the sort order parameter.
func (p *SortOrderParam) GetDefaultValue() interface{} {
	return p.DefaultValue
}
