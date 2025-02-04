/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypeForValue(t *testing.T) {
	tests := []struct {
		in  string
		out TypeID
	}{
		{`true`, BoolID},
		{`TRUE`, BoolID},
		{`True`, BoolID},
		{`t`, DefaultID},
		{`false`, BoolID},
		{`FALSE`, BoolID},
		{`False`, BoolID},
		{`f`, DefaultID},
		{`2018`, IntID},
		{`2018-10`, DateTimeID},
		{`2018-10-03`, DateTimeID},
		{`2018-10-03T20:47:53Z`, DateTimeID},
		{`123`, IntID},
		{`-123`, IntID},
		{`+123`, IntID},
		{`0001`, IntID},
		{`+0`, IntID},
		{`-0`, IntID},
		{`1World`, DefaultID},
		{`3.14159`, FloatID},
		{`-273.15`, FloatID},
		{`2.99792e8`, FloatID},
		{`9.1095E-28`, FloatID},
		{`-.0`, FloatID},
		{`+.0`, FloatID},
		{`.1`, FloatID},
		{`1.`, FloatID},
		{`1-800-4GOLANG`, DefaultID},
		{`+1800-446-5264`, DefaultID},
		{`212.555.9876`, DefaultID},
		{`testing`, DefaultID},
	}
	for _, tc := range tests {
		out, _ := TypeForValue([]byte(tc.in))
		require.Equal(t, tc.out, out, "%s != %s", tc.in, tc.out.Enum())
	}
}

func TestFloatArrayTranslation(t *testing.T) {
	testCases := [][]float32{
		{},
		{0.1},
		{0},
		{0.65433, 1.855, 3.1415926539},
	}
	for _, tc := range testCases {
		asBytes := FloatArrayAsBytes(tc)
		asFloat32 := BytesAsFloatArray(asBytes)
		require.Equal(t, len(tc), len(asFloat32))
		for i := range tc {
			require.Equal(t, tc[i], asFloat32[i])
		}
	}
}
