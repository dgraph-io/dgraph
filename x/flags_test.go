/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package x

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlag(t *testing.T) {
	opt := `bool_key=true; int-key=5; float-key=0.05; string_key=value; ;`
	kvm := parseFlag(opt)
	t.Logf("Got kvm: %+v\n", kvm)
	CheckFlag(opt, "bool-key", "int-key", "string-key", "float-key")
	CheckFlag(opt, "bool-key", "int-key", "string-key", "float-key", "other-key")
	// Has a typo.

	c := func() {
		CheckFlag(opt, "boolo-key", "int-key", "string-key", "float-key")
	}
	require.Panics(t, c)
	require.Equal(t, true, GetFlagBool(opt, "bool-key"))
	require.Equal(t, uint64(5), GetFlagUint64(opt, "int-key"))
	require.Equal(t, "value", GetFlag(opt, "string-key"))
}
