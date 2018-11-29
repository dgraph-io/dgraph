/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

func TestRemoveDuplicates(t *testing.T) {
	set := RemoveDuplicates([]string{"a", "a", "a", "b", "b", "c", "c"})
	require.EqualValues(t, []string{"a", "b", "c"}, set)
}

func TestRemoveDuplicatesWithoutDuplicates(t *testing.T) {
	set := RemoveDuplicates([]string{"a", "b", "c", "d"})
	require.EqualValues(t, []string{"a", "b", "c", "d"}, set)
}

func TestDivideAndRule(t *testing.T) {
	test := func(num, expectedGo, expectedWidth int) {
		numGo, width := DivideAndRule(num)
		require.Equal(t, expectedGo, numGo)
		require.Equal(t, expectedWidth, width)
	}

	test(68, 1, 68)
	test(255, 1, 255)
	test(256, 1, 256)
	test(510, 1, 510)

	test(511, 2, 256)
	test(512, 2, 256)
	test(513, 2, 257)

	test(768, 2, 384)

	test(1755, 4, 439)
}
