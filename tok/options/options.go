/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package options

import (
	"fmt"

	c "github.com/hypermodeinc/dgraph/v25/tok/constraints"
)

// An Options instance maps the various named options to their specific values.
// This is intended as a generic mechanism to specify construction options.
// Note that the value types can be mixed: i.e., we might store both an int
// option value and a string option value in the same map.
type Options map[string]any

// NewOptions() creates a new instance of Options, allowing a client to
// then modify it to specify whichever packaged options desired.
func NewOptions() Options {
	return make(map[string]any)
}

// SetOpt(o, whichOpt, value) will associate whichOpt with value.
func (o Options) SetOpt(whichOpt string, value any) Options {
	o[whichOpt] = value
	return o
}

// GetOpt(o, whichOpt, defaultValue) will attempt to retrieve the
// value associated with whichOpt in o.
// The values returned represent the retrieved (or default) value,
// whether or not the value was found, and
// possibly an error if there was a type conversion error.
// If o does not contain the key whichOpt:
//
//	This returns (defaultValue, false, nil)
//
// If o finds a value for whichOpt, but the type does not match the type for
// defaultValue:
//
//	This returns (defaultValue, true, A Non-Nil-Error)
//
// If o find a value "v" for whichOpt, and the value matches the type for
// defaultValue:
//
//	This returns (v, true, nil).
func GetOpt[T c.Simple](o Options, whichOpt string, defaultValue T) (T, bool, error) {
	val, ok := o[whichOpt]
	if !ok {
		return defaultValue, false, nil
	}
	assigned, ok := val.(T)
	if !ok {
		return defaultValue, true,
			fmt.Errorf("unable to assign a value of type %T to type %T",
				val, defaultValue)
	}
	return assigned, true, nil
}

// GetInterfaceOpt(o, whichOpt) will retrieve the option associated with
// whichOpt from o. If the value is found, this returns the found value and true,
// otherwise, this returns nil and false.
// It is the caller's responsibility to make sure that the option type stored
// is compatible with the variable to which it is being assigned.
// Similarly, it is the client responsibility to handle possible default
// values if the expected value is not found.
func GetInterfaceOpt(o Options, whichOpt string) (any, bool) {
	val, ok := o[whichOpt]
	return val, ok
}

// o.Specifies(whichOpt) returns true if whichOpt has been assigned a value
// in o, and false otherwise.
func (o Options) Specifies(whichOpt string) bool {
	_, found := o[whichOpt]
	return found
}
