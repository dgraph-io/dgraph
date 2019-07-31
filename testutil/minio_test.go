/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	bucketName = "dgraph-test"
)

func TestCreateBucket(t *testing.T) {
	client, err := NewMinioClient()
	require.NoError(t, err)
	if found, _ := client.BucketExists(bucketName); found {
		require.NoError(t, client.RemoveBucket(bucketName))
	}
	require.NoError(t, client.MakeBucket(bucketName, ""))
}
