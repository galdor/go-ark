package json

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.n16f.net/ark/pkg/ark/utils"
)

type Validator struct {
	Pointer jsontext.Pointer
	Errors  Errors
}

type Validatable interface {
	ValidateJSON(v *Validator)
}

func Validate(value any) error {
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
	if token != nil {
		v.Push(pointerTokenString(token))
	}

	fn()

	if token != nil {
		v.Pop()
	}
}

func (v *Validator) AddError(token any, code, format string, args ...any) {
	pointer := v.Pointer
	if token != nil {
		pointer = pointer.AppendToken(pointerTokenString(token))
	}

	err := Error{
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

func (v *Validator) CheckIntMin(token any, i int, min int) bool {
	return v.Check(token, i >= min, "integer_too_small",
		"integer must be greater or equal to %d", min)
}

func (v *Validator) CheckIntMax(token any, i int, max int) bool {
	return v.Check(token, i <= max, "integer_too_large",
		"integer must be lower or equal to %d", max)
}

func (v *Validator) CheckIntMinMax(token any, i int, min, max int) bool {
	if !v.CheckIntMin(token, i, min) {
		return false
	}

	return v.CheckIntMax(token, i, max)
}

func (v *Validator) CheckInt64Min(token any, i, min int64) bool {
	return v.Check(token, i >= min, "integer_too_small",
		"integer must be greater or equal to %d", min)
}

func (v *Validator) CheckInt64Max(token any, i, max int64) bool {
	return v.Check(token, i <= max, "integer_too_large",
		"integer must be lower or equal to %d", max)
}

func (v *Validator) CheckInt64MinMax(token any, i, min, max int64) bool {
	if !v.CheckInt64Min(token, i, min) {
		return false
	}

	return v.CheckInt64Max(token, i, max)
}

func (v *Validator) CheckFloatMin(token any, f, min float64) bool {
	return v.Check(token, f >= min, "float_too_small",
		"float %s must be greater or equal to %s",
		strconv.FormatFloat(f, 'f', -1, 64),
		strconv.FormatFloat(min, 'f', -1, 64))
}

func (v *Validator) CheckFloatMax(token any, f, max float64) bool {
	return v.Check(token, f <= max, "float_too_large",
		"float %s must be lower or equal to %s",
		strconv.FormatFloat(f, 'f', -1, 64),
		strconv.FormatFloat(max, 'f', -1, 64))
}

func (v *Validator) CheckFloatMinMax(token any, f, min, max float64) bool {
	if !v.CheckFloatMin(token, f, min) {
		return false
	}

	return v.CheckFloatMax(token, f, max)
}

func (v *Validator) CheckStringLengthMin(token any, s string, min int) bool {
	length := utf8.RuneCountInString(s)
	return v.Check(token, length >= min, "string_too_short",
		"string length must be greater or equal to %d", min)
}

func (v *Validator) CheckStringLengthMax(token any, s string, max int) bool {
	length := utf8.RuneCountInString(s)
	return v.Check(token, length <= max, "string_too_long",
		"string length must be lower or equal to %d", max)
}

func (v *Validator) CheckStringLengthMinMax(
	token any, s string, min, max int,
) bool {
	if !v.CheckStringLengthMin(token, s, min) {
		return false
	}

	return v.CheckStringLengthMax(token, s, max)
}

func (v *Validator) CheckStringNotEmpty(token any, s string) bool {
	return v.Check(token, s != "", "missing_or_empty_string",
		"missing or empty string")
}

func (v *Validator) CheckURI(token any, s string) bool {
	// The url.Parse function parses URI references. Most of the time we are
	// interested in URIs so we check that there is a scheme.

	uri, err := url.Parse(s)
	if err != nil {
		v.AddError(token, "invalid_uri_format", "string must be a valid URI")
		return false
	}

	if uri.Scheme == "" {
		v.AddError(token, "missing_uri_scheme", "URI must have a scheme")
		return false
	}

	return true
}

func (v *Validator) CheckEmailAddress(token any, s string) bool {
	// Email validation is one of the most nitpicked subject in the software
	// industry. We keep validation to a minimum: one can always write a more
	// stringent method if needs be.

	if !v.CheckStringNotEmpty(token, s) {
		return false
	}

	nbErrors := 0
	addError := func(format string, args ...any) {
		v.AddError(token, "invalid_email_address", format, args...)
		nbErrors++
	}

	localPart, domain, found := strings.Cut(s, "@")
	if found {
		if len(domain) == 0 {
			addError("invalid empty domain")
		}

		if len(localPart) == 0 {
			addError("invalid empty local part")
		}
	} else {
		addError("missing '@' separator")
	}

	return nbErrors == 0
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
		} else if port > 65535 {
			v.AddError(token, "invalid_port_number",
				"port number must be lower or equal to 65535")
			return false
		}
	}

	return true
}

func (v *Validator) CheckOptionalObject(token any, value any) bool {
	if !checkObject(value) {
		return true
	}

	return v.doCheckObject(token, value)
}

func (v *Validator) CheckObject(token any, value any) bool {
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

	v.WithChild(token, func() {
		value2.ValidateJSON(v)
	})

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

func (v *Validator) CheckDirectoryPath(token any, path string) bool {
	if !v.CheckStringNotEmpty(token, path) {
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			v.AddError(token, "file_not_found", "file not found")
			return false
		}

		v.AddError(token, "error", "cannot stat file %v", err)
		return false
	}

	if !info.IsDir() {
		v.AddError(token, "not_a_directory", "file is not a directory")
		return false
	}

	return true
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
