package query

import (
	"fmt"
	"strconv"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo"
	"github.com/pkg/errors"
)

// Define a single validator to do all of the validations for us.
var v = validator.New()

// ValidatedQueryParam extracts a query parameter and validates it.
func ValidatedQueryParam(ctx echo.Context, name, validationTag string) (string, error) {
	value := ctx.QueryParam(name)

	// Validate the value.
	if err := v.Var(value, validationTag); err != nil {
		return "", err
	}

	return value, nil
}

// ValidateBooleanQueryParam extracts a Boolean query parameter and validates it.
func ValidateBooleanQueryParam(ctx echo.Context, name string, defaultValue *bool) (bool, error) {
	errMsg := fmt.Sprintf("invalid query parameter: %s", name)
	value := ctx.QueryParam(name)

	// Assume that the parameter is required if there's no default.
	if defaultValue == nil {
		if err := v.Var(value, "required"); err != nil {
			return false, fmt.Errorf("missing required query parameter: %s", name)
		}
	}

	// If no value was provided at this point then the prameter is optional; return the default value.
	if value == "" {
		return *defaultValue, nil
	}

	// Parse the parameter value and return the result.
	result, err := strconv.ParseBool(value)
	if err != nil {
		return false, errors.Wrap(err, errMsg)
	}
	return result, nil
}

// ValidateUIntQueryParam extracts an integer query parameter and validates it.
func ValidateUIntQueryParam(ctx echo.Context, name string, defaultValue *uint64) (uint64, error) {
	errMsg := fmt.Sprintf("invalid query parameter: %s", name)
	value := ctx.QueryParam(name)

	// Assume that the parameter is required if there is no default.
	if defaultValue == nil {
		if err := v.Var(value, "required"); err != nil {
			return 0, fmt.Errorf("missing required query parameter: %s", name)
		}
	}

	// If no value was provided at this point then the parameter is optional; return the default value.
	if value == "" {
		return *defaultValue, nil
	}

	// Parse the parameter value and return the result.
	result, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, errMsg)
	}
	return result, nil
}

// ValidateBoolPQueryParam extracts and validates an optional Boolean query parameter.
func ValidateBoolPQueryParam(ctx echo.Context, name string) (*bool, error) {
	errMsg := fmt.Sprintf("invalid query parameter: %s", name)
	value := ctx.QueryParam(name)

	// Simply return nil if there's no value.
	if value == "" {
		return nil, nil
	}

	// Parse the parameter value and return the result.
	result, err := strconv.ParseBool(value)
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}
	return &result, nil
}

// ParseableParam represents a data type that can be converted to and from a string.
type ParseableParam interface {
	FromString(string) error
	String() string
	GetValue() interface{}
	GetDefaultValue() interface{}
}

// ValidateParseableParam extracts and validates a parseable query parameter.
func ValidateParseableParam(ctx echo.Context, name string, dest ParseableParam) error {
	errMsg := fmt.Sprintf("invalid query parameter: %s", name)
	value := ctx.QueryParam(name)

	// Assume that the parameter is required if there is no default.
	if dest.GetDefaultValue() == nil {
		if err := v.Var(value, "required"); err != nil {
			return fmt.Errorf("missing required query parameter: %s", name)
		}
	}

	// If no value was provided at this point then the paraemter is optional; return the default value.
	if value == "" {
		return nil
	}

	// Parse the parameter value and return the result.
	err := dest.FromString(value)
	if err != nil {
		return errors.Wrap(err, errMsg)
	}
	return nil
}
