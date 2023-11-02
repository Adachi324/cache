package utils

import (
	"encoding/json"
	"errors"
	"reflect"
)

// GetValue uses reflection to get underlying value
func GetValue(value interface{}) interface{} {
	var valToStore interface{}
	v := DeductPointerVal(value)
	if v.IsValid() {
		valToStore = v.Interface()
	}

	return valToStore
}

// SetValue uses reflection to set underlying value to receiver
func SetValue(val interface{}, receiver interface{}) error {
	receiverValue := DeductPointerVal(receiver)
	if receiverValue.CanSet() {
		if val != nil {
			if reflect.TypeOf(val).AssignableTo(receiverValue.Type()) {
				receiverValue.Set(reflect.ValueOf(val))
			} else {
				return errors.New("cache:value_not_assignable")
			}
		} else {
			receiverValue.Set(reflect.Zero(receiverValue.Type()))
		}
		return nil
	}

	return errors.New("cache:receiver_not_pointer")
}

// SetValueByJSON uses json to set underlying value to receiver
func SetValueByJSON(val interface{}, receiver interface{}) error {
	receiverValue := reflect.ValueOf(receiver)
	if receiverValue.Type().Kind() != reflect.Ptr {
		return errors.New("cache:receiver_not_pointer")
	}
	receiverValue = reflect.ValueOf(receiver).Elem()
	if receiverValue.CanSet() {
		if val != nil {
			valBytes, err := json.Marshal(val)
			if err != nil {
				return errors.New("cache:val_marshal_err")
			}
			err = json.Unmarshal(valBytes, receiver)
			if err != nil {
				return errors.New("cache:val_unmarshal_err")
			}
			return nil
		}
		receiverValue.Set(reflect.Zero(receiverValue.Type()))
		return nil
	}
	return errors.New("cache:receiver_not_pointer")
}
