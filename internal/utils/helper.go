package utils

import "reflect"

// DeductPointerVal gets the underlying value of an interface
func DeductPointerVal(val interface{}) reflect.Value {
	v := reflect.ValueOf(val)
	for v.Kind() == reflect.Ptr {
		if v.IsValid() {
			v = v.Elem()
		} else {
			break
		}
	}

	return v
}
