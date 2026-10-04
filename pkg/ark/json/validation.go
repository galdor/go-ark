package json

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"

	"go.n16f.net/ark/pkg/ark/utils"
)

type ValidationError struct {
	Pointer jsontext.Pointer `json:"pointer"`
	Code    string           `json:"code"`
	Message string           `json:"message"`
}

func (err ValidationError) Error() string {
	if len(err.Pointer) == 0 {
		return err.Message
	} else {
		return fmt.Sprintf("%v  %s", err.Pointer, err.Message)
	}
}

type ValidationErrors []*ValidationError

func (errs ValidationErrors) Error() string {
	var buf bytes.Buffer

	buf.WriteString("invalid data")

	if len(errs) > 0 {
		buf.WriteByte(':')
	}

	for _, err := range errs {
		buf.WriteString("\n  ")
		buf.WriteString(err.Error())
	}

	return buf.String()
}

type Validator struct {
	Pointer jsontext.Pointer
	Errors  ValidationErrors
}

type Validatable interface {
	ValidateJSON(v *Validator)
}

func Validate(value interface{}) error {
	v := NewValidator()

	if validatableValue, ok := value.(Validatable); ok {
		validatableValue.ValidateJSON(v)
	}

	return v.Error()
}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) Error() error {
	if len(v.Errors) == 0 {
		return nil
	}

	return v.Errors
}

func (v *Validator) Push(token any) {
	if token != nil {
		v.Pointer = v.Pointer.AppendToken(pointerTokenString(token))
	}
}

func (v *Validator) Pop() {
	v.Pointer = v.Pointer.Parent()
}

func (v *Validator) WithChild(token any, fn func()) {
	v.Push(pointerTokenString(token))
	defer v.Pop()

	fn()
}

func (v *Validator) AddError(token any, code, format string, args ...any) {
	pointer := v.Pointer
	if token != nil {
		pointer = pointer.AppendToken(pointerTokenString(token))
	}

	err := ValidationError{
		Pointer: pointer,
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}

	v.Errors = append(v.Errors, &err)
}

func (v *Validator) Check(
	token any, value bool, code, format string, args ...any,
) bool {
	if !value {
		v.AddError(token, code, format, args...)
	}

	return value
}

func (v *Validator) CheckStringNotEmpty(token any, s string) bool {
	return v.Check(token, s != "", "missing_or_empty_string",
		"missing or empty string")
}

func (v *Validator) CheckNetworkAddress(token any, s string) bool {
	_, portString, err := net.SplitHostPort(s)
	if err != nil {
		var msg string
		var addrErr *net.AddrError

		if errors.As(err, &addrErr) {
			msg = addrErr.Err
		} else {
			msg = err.Error()
		}

		v.AddError(token, "invalid_address", "invalid address: %v", msg)
		return false
	}

	if portString == "" {
		v.AddError(token, "empty_port_number", "empty port number")
		return false
	} else {
		port, err := strconv.ParseInt(portString, 10, 64)
		if err != nil {
			v.AddError(token, "invalid_port_number", "invalid port number")
			return false
		} else if port < 1 {
			v.AddError(token, "invalid_port_number",
				"port number must be greater than 0")
			return false
		} else if port >= 65535 {
			v.AddError(token, "invalid_port_number",
				"port number must be lower than 65535")
			return false
		}
	}

	return true
}

func (v *Validator) CheckOptionalObject(token interface{}, value interface{}) bool {
	if !checkObject(value) {
		return true
	}

	return v.doCheckObject(token, value)
}

func (v *Validator) CheckObject(token interface{}, value interface{}) bool {
	if !checkObject(value) {
		v.AddError(token, "missing_or_null_value", "missing or null value")
		return false
	}

	return v.doCheckObject(token, value)
}

func (v *Validator) doCheckObject(token, value any) bool {
	nbErrors := len(v.Errors)

	value2, ok := value.(Validatable)
	if !ok {
		return true
	}

	v.Push(token)
	value2.ValidateJSON(v)
	v.Pop()

	return len(v.Errors) == nbErrors
}

func checkObject(value any) bool {
	valueType := reflect.TypeOf(value)
	if valueType == nil {
		return false
	}

	if valueType.Kind() != reflect.Pointer {
		utils.Panic("value %#v (%T) is not a pointer", value, value)
	}

	pointedValueType := valueType.Elem()
	if pointedValueType.Kind() != reflect.Struct {
		utils.Panic("value %#v (%T) is not a pointer to a structure",
			value, value)
	}

	return !reflect.ValueOf(value).IsZero()
}

func pointerTokenString(token any) string {
	if token == nil {
		return ""
	}

	var s string

	switch value := token.(type) {
	case string:
		s = value
	case int:
		s = strconv.Itoa(value)
	default:
		utils.Panic("invalid token %#v (%T)", token, token)
	}

	return s
}
