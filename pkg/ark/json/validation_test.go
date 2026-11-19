package json

import (
	"encoding/json/jsontext"
	"testing"

	"go.n16f.net/check"
)

type testObject struct {
	String string
}

func (obj *testObject) ValidateJSON(v *Validator) {
	v.CheckStringNotEmpty("string", obj.String)
}

func TestCheckStringNotEmpty(t *testing.T) {
	check := check.New(t)

	var v *Validator

	v = NewValidator()
	if check.False(v.CheckStringNotEmpty(nil, "")) {
		checkValidationError(t, v, 0, "", "missing_or_empty_string")
	}

	v = NewValidator()
	check.True(v.CheckStringNotEmpty(nil, "foo"))
}

func TestCheckNetworkAddress(t *testing.T) {
	check := check.New(t)

	var v *Validator

	v = NewValidator()
	if check.False(v.CheckNetworkAddress(nil, "")) {
		checkValidationError(t, v, 0, "", "invalid_address")
	}

	v = NewValidator()
	if check.False(v.CheckNetworkAddress(nil, "localhost")) {
		checkValidationError(t, v, 0, "", "invalid_address")
	}

	v = NewValidator()
	if check.False(v.CheckNetworkAddress(nil, "localhost:foo")) {
		checkValidationError(t, v, 0, "", "invalid_port_number")
	}

	v = NewValidator()
	check.True(v.CheckNetworkAddress(nil, "[::1]:80"))

	v = NewValidator()
	check.True(v.CheckNetworkAddress(nil, "localhost:8080"))
}

func TestValidationPaths(t *testing.T) {
	obj := testObject{}

	v := NewValidator()
	v.CheckObject(nil, &obj)

	checkValidationError(t, v, 0, "/string", "missing_or_empty_string")
}

func checkValidationError(
	t *testing.T, v *Validator, i int, pointer jsontext.Pointer, code string,
) bool {
	t.Helper()

	check := check.New(t)

	if check.Lesser(i, len(v.Errors)) {
		validationErr := v.Errors[i]

		return check.Equal(validationErr.Pointer, pointer) &&
			check.Equal(validationErr.Code, code)
	}

	return true
}
