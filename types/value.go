/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package types

import (
	"strconv"
	"strings"
)

// TypeForValue tries to determine the most likely type based on a value. We only want to use this
// function when there's no schema type and no suggested storage type.
// Returns the guessed type or DefaultID if it couldn't be determined.
// If retval is non-nil, the parsed value is returned, useful in conjunction with ObjectValue().
func TypeForValue(v []byte) (TypeID, interface{}) {
	s := string(v)
	switch {
	case v == nil || s == "":
		break

	// Possible boolean. Specific to "true" or "false".
	case s[0] == 't', s[0] == 'T', s[0] == 'f', s[0] == 'F':
		var b bool
		// XXX: we dont use ParseBool here because it considers 't' and 'f' as values.
		switch s {
		case "true", "TRUE", "True":
			b = true
			return BoolID, b
		case "false", "FALSE", "False":
			return BoolID, b
		}

	// Possible datetime. Unfortunately, year-only value will fallthrough as int.
	case checkDateTime(s):
		if t, err := ParseTime(s); err == nil {
			return DateTimeID, t
		}

	// Possible int.
	case checkInt(s):
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return IntID, i
		}

	// Possible float.
	case checkFloat(s):
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return FloatID, f
		}
	}
	return DefaultID, nil
}

func isSign(d byte) bool {
	return d == '-' || d == '+'
}

func isDigit(d byte) bool {
	return d >= '0' && d <= '9'
}

func checkInt(s string) bool {
	if isSign(s[0]) && len(s) > 1 {
		s = s[1:]
	}
	return isDigit(s[0]) && !strings.ContainsAny(s[1:], ".Ee")
}

func checkFloat(s string) bool {
	if isSign(s[0]) && len(s) > 1 {
		s = s[1:]
	}
	if s[0] == '.' && len(s) > 1 {
		// .012 is totally legit
		return isDigit(s[1])
	}
	return isDigit(s[0]) && strings.ContainsAny(s[1:], ".Ee")
}

func checkDateTime(s string) bool {
	if len(s) < 5 {
		return false
	}
	return isDigit(s[0]) && isDigit(s[1]) && isDigit(s[2]) && isDigit(s[3]) && s[4] == '-'
}
