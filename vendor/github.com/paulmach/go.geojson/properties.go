package geojson

import (
	"fmt"
)

// SetProperty provides the inverse of all the property functions
// and is here for consistency.
func (f *Feature) SetProperty(key string, value interface{}) {
	if f.Properties == nil {
		f.Properties = make(map[string]interface{})
	}
	f.Properties[key] = value
}

// PropertyBool type asserts a property to `bool`.
func (f *Feature) PropertyBool(key string) (bool, error) {
	if b, ok := (f.Properties[key]).(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("type assertion of `%s` to bool failed", key)
}

// PropertyInt type asserts a property to `int`.
func (f *Feature) PropertyInt(key string) (int, error) {
	if i, ok := (f.Properties[key]).(int); ok {
		return i, nil
	}

	if i, ok := (f.Properties[key]).(float64); ok {
		return int(i), nil
	}

	return 0, fmt.Errorf("type assertion of `%s` to int failed", key)
}

// PropertyFloat64 type asserts a property to `float64`.
func (f *Feature) PropertyFloat64(key string) (float64, error) {
	if i, ok := (f.Properties[key]).(float64); ok {
		return i, nil
	}
	return 0, fmt.Errorf("type assertion of `%s` to float64 failed", key)
}

// PropertyString type asserts a property to `string`.
func (f *Feature) PropertyString(key string) (string, error) {
	if s, ok := (f.Properties[key]).(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("type assertion of `%s` to string failed", key)
}

// PropertyMustBool guarantees the return of a `bool` (with optional default)
//
// useful when you explicitly want a `bool` in a single value return context:
//     myFunc(f.PropertyMustBool("param1"), f.PropertyMustBool("optional_param", true))
func (f *Feature) PropertyMustBool(key string, def ...bool) bool {
	var defaul bool

	b, err := f.PropertyBool(key)
	if err == nil {
		return b
	}

	if len(def) > 0 {
		defaul = def[0]
	}

	return defaul
}

// PropertyMustInt guarantees the return of a `bool` (with optional default)
//
// useful when you explicitly want a `bool` in a single value return context:
//     myFunc(f.PropertyMustInt("param1"), f.PropertyMustInt("optional_param", 123))
func (f *Feature) PropertyMustInt(key string, def ...int) int {
	var defaul int

	b, err := f.PropertyInt(key)
	if err == nil {
		return b
	}

	if len(def) > 0 {
		defaul = def[0]
	}

	return defaul
}

// PropertyMustFloat64 guarantees the return of a `bool` (with optional default)
//
// useful when you explicitly want a `bool` in a single value return context:
//     myFunc(f.PropertyMustFloat64("param1"), f.PropertyMustFloat64("optional_param", 10.1))
func (f *Feature) PropertyMustFloat64(key string, def ...float64) float64 {
	var defaul float64

	b, err := f.PropertyFloat64(key)
	if err == nil {
		return b
	}

	if len(def) > 0 {
		defaul = def[0]
	}

	return defaul
}

// PropertyMustString guarantees the return of a `bool` (with optional default)
//
// useful when you explicitly want a `bool` in a single value return context:
//     myFunc(f.PropertyMustString("param1"), f.PropertyMustString("optional_param", "default"))
func (f *Feature) PropertyMustString(key string, def ...string) string {
	var defaul string

	b, err := f.PropertyString(key)
	if err == nil {
		return b
	}

	if len(def) > 0 {
		defaul = def[0]
	}

	return defaul
}
