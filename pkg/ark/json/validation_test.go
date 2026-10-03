package json

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testObject struct {
	String string
}

func (obj *testObject) ValidateJSON(v *Validator) {
	v.CheckStringNotEmpty("string", obj.String)
}

func TestCheckStringNotEmpty(t *testing.T) {
	assert := assert.New(t)

	var v *Validator

	v = NewValidator()
	if assert.False(v.CheckStringNotEmpty(nil, "")) {
		assertValidationError(t, v, 0, "", "missing_or_empty_string")
	}

	v = NewValidator()
	assert.True(v.CheckStringNotEmpty(nil, "foo"))
}

func TestCheckNetworkAddress(t *testing.T) {
	assert := assert.New(t)

	var v *Validator

	v = NewValidator()
	if assert.False(v.CheckNetworkAddress(nil, "")) {
		assertValidationError(t, v, 0, "", "invalid_address")
	}

	v = NewValidator()
	if assert.False(v.CheckNetworkAddress(nil, "localhost")) {
		assertValidationError(t, v, 0, "", "invalid_address")
	}

	v = NewValidator()
	if assert.False(v.CheckNetworkAddress(nil, "localhost:foo")) {
		assertValidationError(t, v, 0, "", "invalid_port_number")
	}

	v = NewValidator()
	assert.True(v.CheckNetworkAddress(nil, "[::1]:80"))

	v = NewValidator()
	assert.True(v.CheckNetworkAddress(nil, "localhost:8080"))
}

func TestValidationPaths(t *testing.T) {
	obj := testObject{}

	v := NewValidator()
	v.CheckObject(nil, &obj)

	assertValidationError(t, v, 0, "/string", "missing_or_empty_string")
}

func assertValidationError(
	t *testing.T, v *Validator, i int, pointer jsontext.Pointer, code string,
) bool {
	t.Helper()

	if assert.Less(t, i, len(v.Errors)) {
		validationErr := v.Errors[i]

		return assert.Equal(t, pointer, validationErr.Pointer) &&
			assert.Equal(t, code, validationErr.Code)
	}

	return true
}
