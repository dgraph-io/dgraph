/* Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package protos

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
)

func TestProtosRegenerate(t *testing.T) {
	err := testutil.Exec("make", "regenerate")
	require.NoError(t, err, "Got error while regenerating protos: %v\n", err)

	generatedProtos := filepath.Join("pb", "pb.pb.go")
	err = testutil.Exec("git", "diff", "--quiet", "--", generatedProtos)
	require.NoError(t, err, "pb.pb.go changed after regenerating")
}
