package security

import (
	"reflect"
	"strings"
)

// RedactPasswords walks through any struct and replaces values of fields containing "password" with "********"
func RedactPasswords(cfg any) any {
	val := reflect.ValueOf(cfg)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	copy := reflect.New(val.Type()).Elem()
	copy.Set(val)
	redactFields(copy)
	return copy.Interface()
}

func redactFields(v reflect.Value) {
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := v.Type().Field(i)

		if field.Kind() == reflect.Struct {
			redactFields(field)
		} else if strings.Contains(strings.ToLower(fieldType.Name), "password") {
			if field.Kind() == reflect.String && field.CanSet() {
				field.SetString("********")
			}
		}
	}
}
