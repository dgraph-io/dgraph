// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package backup

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetHandler(t *testing.T) {
	tests := []struct {
		in  string
		out handler
	}{
		{in: "file", out: &fileHandler{}},
		{in: "minio", out: &s3Handler{}},
		{in: "s3", out: &s3Handler{}},
		{in: "", out: &fileHandler{}},
		{in: "something", out: nil},
	}
	for _, tc := range tests {
		actual := getHandler(tc.in)
		require.Equal(t, tc.out, actual)
	}
}
