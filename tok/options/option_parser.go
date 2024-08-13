/*
 * Copyright 2023 DGraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package options

import (
	"strconv"
)

// OptionParser translates a string to a value (assuming that the option
// value is well-formed). See AllowedOptions to see the purpose.
type OptionParser func(optValue string) (any, error)

// TODO: Provide other built-in OptionParser implementations for other
//       simple types.

// IntOptParser implements OptionParser specifically translating
// int options. It assumes decimal representation of the value.
func IntOptParser(optValue string) (any, error) {
	return strconv.Atoi(optValue)
}

// UintOptParser implement OptionParser specifically translating
// uint options. It assumes decimal representation of the value.
func UintOptParser(optValue string) (any, error) {
	// Specified originally as 64 bit for conversion, since it is
	// not trivial (as far as I can tell) to determine the
	// uint bit size. Simpler to just get it back as 64 bit then
	// cast it.
	retVal, err := strconv.ParseUint(optValue, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(retVal), nil
}

// StringOptionParser implements OptionParser specifically translating
// string options (well, it actually just passes in the string value as-is with
// no modification, but having this allows us to simply implement the
// OptionParser interface.
func StringOptParser(optValue string) (any, error) {
	return optValue, nil
}

// Float64OptParser implements OptionParser specifically translating
// float64 options.
func Float64OptParser(optValue string) (any, error) {
	return strconv.ParseFloat(optValue, 64)
}

// Float32OptParser implements OptionParser specifically translating
// float32 options.
func Float32OptParser(optValue string) (any, error) {
	return strconv.ParseFloat(optValue, 32)
}
