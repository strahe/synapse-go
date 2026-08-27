package ifaceutil

import "reflect"

// NormalizeNil converts nil-like interface values to the zero value of T.
func NormalizeNil[T any](value T) T {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return value
	}
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if rv.IsNil() {
			var zero T
			return zero
		}
	}
	return value
}
