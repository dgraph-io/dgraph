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
		in  []byte
		out TypeID
	}{
		{[]byte(`true`), BoolID},
		{[]byte(`TRUE`), BoolID},
		{[]byte(`True`), BoolID},
		{[]byte(`t`), DefaultID},
		{[]byte(`false`), BoolID},
		{[]byte(`FALSE`), BoolID},
		{[]byte(`False`), BoolID},
		{[]byte(`f`), DefaultID},
		{[]byte(`2018`), IntID},
		{[]byte(`2018-10`), DateTimeID},
		{[]byte(`2018-10-03`), DateTimeID},
		{[]byte(`2018-10-03T20:47:53Z`), DateTimeID},
		{[]byte(`123`), IntID},
		{[]byte(`-123`), IntID},
		{[]byte(`+123`), IntID},
		{[]byte(`0001`), IntID},
		{[]byte(`+0`), IntID},
		{[]byte(`-0`), IntID},
		{[]byte(`1World`), DefaultID},
		{[]byte(`3.14159`), FloatID},
		{[]byte(`-273.15`), FloatID},
		{[]byte(`2.99792e8`), FloatID},
		{[]byte(`9.1095E-28`), FloatID},
		{[]byte(`-.0`), FloatID},
		{[]byte(`+.0`), FloatID},
		{[]byte(`.1`), FloatID},
		{[]byte(`1.`), FloatID},
		{[]byte(`1-800-4GOLANG`), DefaultID},
		{[]byte(`+1800-446-5264`), DefaultID},
		{[]byte(`212.555.9876`), DefaultID},
		{[]byte(`testing`), DefaultID},
	}
	for _, tc := range tests {
		out, _ := TypeForValue(tc.in)
		require.Equal(t, tc.out, out, "%s != %s", string(tc.in), tc.out.Enum())
	}
}
