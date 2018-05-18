// Copyright 2018 Roman Strashkin.  All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package validate

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const VALIDATE_TAG_NAME = "validate"

const (
	TAG_FIELD_NAME     = "name"
	TAG_FIELD_REQUIRED = "required"
	TAG_FIELD_MAX      = "max"
	TAG_FIELD_MIN      = "min"
	TAG_FIELD_MAX_LEN  = "maxLen"
	TAG_FIELD_MIN_LEN  = "minLen"
	TAG_FIELD_DEFAULT  = "default"
	TAG_FIELD_ONE_OF   = "oneof"
)

const (
	VALIDATE_ERR_CODE_UNKNOWN = iota
	VALIDATE_ERR_CODE_MISSING_REQ_PARAM
	VALIDATE_ERR_CODE_UNPARSABLE
	VALIDATE_ERR_CODE_TOO_LONG
	VALIDATE_ERR_CODE_TOO_SHORT
	VALIDATE_ERR_CODE_TOO_BIG
	VALIDATE_ERR_CODE_TOO_SMALL
	VALIDATE_ERR_CODE_INVALID
)

type ValidateError struct {
	Code          int
	ParamName     string
	OriginalError error
}

func (e *ValidateError) Error() string {
	if e.OriginalError == nil {
		switch e.Code {
		case VALIDATE_ERR_CODE_UNPARSABLE:
			return fmt.Sprintf("Param '%s' is invalid or corrupted", e.ParamName)
		}
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

		fValue := outValue.Elem().FieldByName(structField.Name)
		vParams := decodeTagFields(tagFieldsRaw)

		val, ok := inputData[vParams.Name]
		if !ok {
			if vParams.Required {
				return &ValidateError{
					ParamName:     vParams.Name,
					Code:          VALIDATE_ERR_CODE_MISSING_REQ_PARAM,
					OriginalError: fmt.Errorf("Param '%s' is required", vParams.Name),
				}
			}

			if v, ok := vParams.Fields[TAG_FIELD_DEFAULT]; ok {
				setDefaultValue(fValue.Addr(), v)
			} else {
				continue
			}
		} else {
			errDecode := json.Unmarshal(*val, fValue.Addr().Interface())
			if errDecode != nil {
				return &ValidateError{
					ParamName:     vParams.Name,
					Code:          VALIDATE_ERR_CODE_UNPARSABLE,
					OriginalError: errDecode,
				}
			}
		}

		for tagName, tagRawVal := range vParams.Fields {
			switch tagName {
			case TAG_FIELD_MIN, TAG_FIELD_MAX:
				valErr := ValidateError{
					ParamName:     vParams.Name,
					Code:          VALIDATE_ERR_CODE_UNKNOWN,
					OriginalError: nil,
				}
				var val interface{}
				var err error
				switch fValue.Kind() {
				case reflect.Int, reflect.Int8, reflect.Int32, reflect.Int64:
					val, err = strconv.ParseInt(tagRawVal, 10, 64)
					if err != nil {
						panic(fmt.Sprintf("Unable to parse '%s' tag as a signed integer", tagName))
					}

					if tagName == TAG_FIELD_MIN && fValue.Int() < val.(int64) {
						valErr.Code = VALIDATE_ERR_CODE_TOO_SMALL
					}

					if tagName == TAG_FIELD_MAX && fValue.Int() > val.(int64) {
						valErr.Code = VALIDATE_ERR_CODE_TOO_BIG
					}
				case reflect.Uint, reflect.Uint8, reflect.Uint32, reflect.Uint64:
					val, err = strconv.ParseUint(tagRawVal, 10, 64)
					if err != nil {
						panic(fmt.Sprintf("Unable to parse '%s' tag as a unsigned integer", tagName))
					}

					if tagName == TAG_FIELD_MIN && fValue.Uint() < val.(uint64) {
						valErr.Code = VALIDATE_ERR_CODE_TOO_SMALL
					}

					if tagName == TAG_FIELD_MAX && fValue.Uint() > val.(uint64) {
						valErr.Code = VALIDATE_ERR_CODE_TOO_BIG
					}
				case reflect.Float32, reflect.Float64:
					val, err = strconv.ParseFloat(tagRawVal, 64)
					if err != nil {
						panic(fmt.Sprintf("Unable to parse default (%s) as a float: %s", tagRawVal, err.Error()))
					}

					if tagName == TAG_FIELD_MIN && fValue.Float() < val.(float64) {
						valErr.Code = VALIDATE_ERR_CODE_TOO_SMALL
					}

					if tagName == TAG_FIELD_MAX && fValue.Float() > val.(float64) {
						valErr.Code = VALIDATE_ERR_CODE_TOO_BIG
					}
				default:
					panic(fmt.Sprintf("Tag '%s' cannot be applied to field '%s'. "+
						"The field is not an integer or float", tagName, structField.Name))
				}

				switch valErr.Code {
				case VALIDATE_ERR_CODE_TOO_SMALL:
					valErr.OriginalError = fmt.Errorf("Param '%s' is too small (< %v)", valErr.ParamName, val)
				case VALIDATE_ERR_CODE_TOO_BIG:
					valErr.OriginalError = fmt.Errorf("Param '%s' is too big (> %v)", valErr.ParamName, val)
				}
			case TAG_FIELD_MIN_LEN, TAG_FIELD_MAX_LEN:
				if fValue.Kind() != reflect.String {
					panic(fmt.Sprintf("Tag '%s' cannot be applied to field '%s'. "+
						"The field is not a string", tagName, structField.Name))
				}

				reqLen, err := strconv.ParseUint(tagRawVal, 10, 64)
				if err != nil {
					panic(fmt.Sprintf("Unable to parse '%s' tag as an unsigned integer", tagName))
				}

				if tagName == TAG_FIELD_MAX_LEN && len(fValue.String()) > int(reqLen) {
					return &ValidateError{
						ParamName:     vParams.Name,
						Code:          VALIDATE_ERR_CODE_TOO_LONG,
						OriginalError: fmt.Errorf("Param '%s' is too long (> %d)", vParams.Name, reqLen),
					}
				}

				if tagName == TAG_FIELD_MIN_LEN && len(fValue.String()) < int(reqLen) {
					return &ValidateError{
						ParamName:     vParams.Name,
						Code:          VALIDATE_ERR_CODE_TOO_SHORT,
						OriginalError: fmt.Errorf("Param '%s' is too short (< %d)", vParams.Name, reqLen),
					}
				}
			case TAG_FIELD_ONE_OF:

			case TAG_FIELD_DEFAULT:
				// This tag already processed
			default:
				panic(fmt.Sprintf("Unknown tag field: '%s'", tagName))
			}
		}
	}

	return nil
}

func setDefaultValue(fieldPtr reflect.Value, rawValue string) {

	field := reflect.Indirect(fieldPtr)
	kind := field.Kind()
	switch kind {
	case reflect.Uint:
		val, err := strconv.ParseUint(rawValue, 10, 64)
		if err != nil {
			panic(fmt.Sprintf("Unable to parse default (%s) as an unsigned integer: %s", rawValue, err.Error()))
		}

		field.SetUint(val)
	case reflect.Int:
		val, err := strconv.ParseInt(rawValue, 10, 64)
		if err != nil {
			panic(fmt.Sprintf("Unable to parse default (%s) as a signed integer: %s", rawValue, err.Error()))
		}

		field.SetInt(val)
	case reflect.Float32:
		val, err := strconv.ParseFloat(rawValue, 32)
		if err != nil {
			panic(fmt.Sprintf("Unable to parse default (%s) as a float32: %s", rawValue, err.Error()))
		}

		field.SetFloat(float64(val))
	case reflect.Float64:
		val, err := strconv.ParseFloat(rawValue, 64)
		if err != nil {
			panic(fmt.Sprintf("Unable to parse default (%s) as a float64: %s", rawValue, err.Error()))
		}

		field.SetFloat(val)
	case reflect.String:
		field.SetString(rawValue)
	default:
		panic(fmt.Sprintf("Unsupported kind of value: %s", kind.String()))
	}
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
