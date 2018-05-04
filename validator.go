// Copyright 2018 Roman Strashkin.  All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package validate

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const VALIDATE_TAG_NAME = "validate"

const (
	TAG_FIELD_NAME     = "name"
	TAG_FIELD_REQUIRED = "required"
)

const (
	VALIDATE_ERR_CODE_UNKNOWN = iota
	VALIDATE_ERR_CODE_MISSING_REQ_PARAM
	VALIDATE_ERR_CODE_UNPARSABLE
)

type ValidateError struct {
	Code          int
	ParamName     string
	OriginalError error
}

func (e *ValidateError) Error() string {
	if e.OriginalError == nil {
		return ""
	}

	return e.OriginalError.Error()
}

type FieldValidationParams struct {
	Name     string
	Required bool
	Fields   map[string]string
}

func Validate(inputData map[string]*json.RawMessage, outputStruct interface{}) error {
	outValue := reflect.ValueOf(outputStruct)
	if outValue.Kind() != reflect.Ptr {
		panic("input argument is not a pointer")
	}

	if outValue.IsNil() {
		panic("input argument is a nil pointer")
	}

	outType := outValue.Elem().Type()
	if outType.Kind() != reflect.Struct {
		panic("input argument should be a poiner to a struct")
	}

	for i := 0; i < outType.NumField(); i++ {
		structField := outType.Field(i)
		tagValue, ok := structField.Tag.Lookup(VALIDATE_TAG_NAME)
		if !ok {
			continue
		}

		tagFieldsRaw := strings.Split(tagValue, ",")
		if len(tagFieldsRaw) == 0 {
			panic(fmt.Sprintf("Field '%s': empty tag", structField.Name))
		}

		vParams := decodeTagFields(tagFieldsRaw)

		val, ok := inputData[vParams.Name]
		if !ok {
			if vParams.Required {
				return &ValidateError{
					ParamName:     vParams.Name,
					Code:          VALIDATE_ERR_CODE_MISSING_REQ_PARAM,
					OriginalError: nil,
				}
			}

			continue
		}

		fValue := outValue.Elem().FieldByName(structField.Name)
		if fValue.Kind() != reflect.Ptr {
			fValue = fValue.Addr()
		}

		errDecode := json.Unmarshal(*val, fValue.Interface())
		if errDecode != nil {
			return &ValidateError{
				ParamName:     vParams.Name,
				Code:          VALIDATE_ERR_CODE_UNPARSABLE,
				OriginalError: errDecode,
			}
		}

		for name, _ := range vParams.Fields {
			switch name {
			default:
				panic(fmt.Sprintf("Unknown tag field: '%s'", name))
			}
		}
	}

	return nil
}

func decodeTagFields(tagFieldsRaw []string) FieldValidationParams {
	vParams := FieldValidationParams{
		Name:     "",
		Required: false,
		Fields:   make(map[string]string),
	}

	seen := make(map[string]bool)

	for _, v := range tagFieldsRaw {
		splitRes := strings.SplitN(v, "=", 2)
		if splitRes[0] == "" {
			panic("Empty tag field")
		}

		tagFieldValue := ""
		if len(splitRes) == 2 {
			tagFieldValue = splitRes[1]
		}

		if _, ok := seen[splitRes[0]]; ok {
			panic(fmt.Sprintf("Tag field '%s' aleady defined", splitRes[0]))
		}

		seen[splitRes[0]] = true

		switch splitRes[0] {
		case TAG_FIELD_NAME:
			if tagFieldValue == "" {
				panic(fmt.Sprintf("Tag field '%s' is empty", TAG_FIELD_NAME))
			}

			vParams.Name = tagFieldValue
		case TAG_FIELD_REQUIRED:
			switch tagFieldValue {
			case "":
				fallthrough
			case "true":
				vParams.Required = true
			case "false":
				vParams.Required = false
			default:
				panic(fmt.Sprintf("Tag field '%s' has unknown value", TAG_FIELD_REQUIRED))
			}
		default:
			vParams.Fields[splitRes[0]] = tagFieldValue
		}
	}

	return vParams
}
