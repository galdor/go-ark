package json

import (
	"bytes"
	"cmp"
	"encoding/json/jsontext"
	json2 "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
)

type Error struct {
	Pointer jsontext.Pointer `json:"pointer"`
	Code    string           `json:"code"`
	Message string           `json:"message"`
}

func (err Error) Error() string {
	if len(err.Pointer) == 0 {
		return err.Message
	} else {
		return fmt.Sprintf("%v %s", err.Pointer, err.Message)
	}
}

type Errors []*Error

func (errs Errors) Error() string {
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

func TranslateError(err error) error {
	var syntaxErr *jsontext.SyntacticError
	var semErr *json2.SemanticError

	if errors.As(err, &syntaxErr) {
		return TranslateSyntaxicError(syntaxErr)
	} else if errors.As(err, &semErr) {
		return TranslateSemanticError(semErr)
	}

	return err
}

func TranslateSyntaxicError(err *jsontext.SyntacticError) error {
	ptr := err.JSONPointer

	err2 := Error{
		Pointer: ptr,
		Code:    "invalid_data",
		Message: err.Err.Error(),
	}

	return Errors{&err2}
}

func TranslateSemanticError(err *json2.SemanticError) error {
	ptr := err.JSONPointer

	err2 := Error{
		Pointer: ptr,
	}

	if errors.Is(err.Err, json2.ErrUnknownName) {
		err2.Code = "unknown_member"
		err2.Message = fmt.Sprintf("unknown member %q", ptr.LastToken())
	} else if err.Err == nil && err.JSONKind != 0 && err.GoType != nil {
		err2.Code = "invalid_value"
		jsonName := jsonKindName(err.JSONKind)
		if jsonName == "" {
			jsonName = "value"
		}
		goName := goTypeName(err.GoType)
		err2.Message = fmt.Sprintf("cannot convert JSON %s to %s",
			jsonName, goName)
	} else {
		err2.Code = "invalid_data"
		if err.Err != nil {
			err2.Message = err.Err.Error()
		} else {
			err2.Message = "invalid value"
		}
	}

	return Errors{&err2}
}

func jsonKindName(kind jsontext.Kind) string {
	var s string

	switch kind {
	case jsontext.KindNull:
		s = "null"
	case jsontext.KindTrue, jsontext.KindFalse:
		s = "boolean"
	case jsontext.KindString:
		s = "string"
	case jsontext.KindNumber:
		s = "number"
	case jsontext.KindBeginObject, jsontext.KindEndObject:
		s = "object"
	case jsontext.KindBeginArray, jsontext.KindEndArray:
		s = "array"
	}

	return s
}

func goTypeName(t reflect.Type) string {
	if t == nil {
		return "value"
	}

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	var s string

	switch t.Kind() {
	case reflect.Bool:
		s = "boolean"

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64,
		reflect.Float32, reflect.Float64:
		s = "number"

	case reflect.String:
		s = "string"

	case reflect.Slice, reflect.Array:
		s = "array"

	case reflect.Map, reflect.Struct:
		s = "object"

	default:
		s = cmp.Or(t.Name(), t.String())
	}

	return s
}
