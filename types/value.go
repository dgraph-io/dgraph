/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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
	"encoding/binary"
	"math"
	"strconv"
	"strings"
)

// BytesAsFloatArray(encoded) converts encoded into a []float64.
// If len(encoded) % 8 is not 0, it will ignore any trailing
// bytes, and simply convert 8 bytes at a time to generate the
// float64 entries.
// WARNING: Current implementation always requires a memory allocation!
func BytesAsFloatArray(encoded []byte) []float64 {
	// Unfortunately, this is not as simple as casting the result,
	// and it is also not possible to directly use the
	// golang "unsafe" library to directly do the conversion.
	// The operation:
	//      []float64(encoded)  does not compile!
	// Whereas:
	//      []float64(unsafe.Slice(unsafe.Ptr(encoded), len(encoded)))
	// might compile (actually have not tested it), but its success or
	// failure depends on agreement of the serialization mechanism
	// of the source data and the data as it exists on the machine where
	// the operation is being performed.
	// The machine where this operation gets run might prefer
	// BigEndian/LittleEndian, but the machine that sent it may have
	// preferred the other, and there is no way to tell!
	//
	// The solution below, unfortunately, requires another memory
	// allocation.
	// TODO Potential optimization: If we detect that current machine is
	// using LittleEndian format, there might be a way of making this
	// work with the golang "unsafe" library.

	resultLen := len(encoded) / 8
	if resultLen == 0 {
		return []float64{}
	}
	retVal := make([]float64, resultLen)
	for i := 0; i < resultLen; i++ {
		// Assume LittleEndian for encoding since this is
		// the assumption elsewhere when reading from client.
		// See dgraph-io/dgo/protos/api.pb.go
		// See also dgraph-io/dgraph/types/conversion.go
		// This also seems to be the preference from many examples
		// I have found via Google search. It's unclear why this
		// should be a preference.
		bits := binary.LittleEndian.Uint64(encoded)
		retVal[i] = math.Float64frombits(bits)
		encoded = encoded[8:]
	}
	return retVal
}

// FloatArrayAsBytes(v) will create a byte array encoding
// v using LittleEndian format. This is sort of the inverse
// of BytesAsFloatArray, but note that we can always be successful
// converting to bytes, but the inverse is not feasible.
func FloatArrayAsBytes(v []float64) []byte {
	retVal := make([]byte, 8*len(v))
	offset := retVal
	for i := 0; i < len(v); i++ {
		bits := math.Float64bits(v[i])
		binary.LittleEndian.PutUint64(offset, bits)
		offset = offset[8:]
	}
	return retVal
}

func FloatArrayAsString(v []float64) string {
	retVal := "["
	for i := range v {
		retVal += strconv.FormatFloat(v[i], 'f', -1, 64)
		if i != len(v)-1 {
			retVal += ", "
		}
	}
	retVal += "]"
	return retVal
}

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

	// TODO: Consider looking for vector type (vfloat). Not clear
	//       about this, because the natural encoding as
	//       "[ num, num, ... num ]" my look like a standard list type.
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
