/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
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
