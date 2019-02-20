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

func TestValidateAddress(t *testing.T) {
	t.Run("IPv4", func(t *testing.T) {
		testData := []struct {
			name    string
			address string
			isValid bool
		}{
			{"Valid without port", "190.0.0.1", false},
			{"Valid with port", "192.5.32.1:333", true},
			{"Invalid without port", "12.0.0", false},
			// the following test returns true because 12.0.0 is considered as valid
			// hostname
			{"Invalid with port", "12.0.0:3333", true},
		}
		for _, subtest := range testData {
			st := subtest
			t.Run(st.name, func(t *testing.T) {
				require.Equal(t, st.isValid, ValidateAddress(st.address))
			})
		}

	})
	t.Run("IPv6", func(t *testing.T) {
		testData := []struct {
			name    string
			address string
			isValid bool
		}{
			{"Valid without port", "[2001:db8::1]", false},
			{"Valid with port", "[2001:db8::1]:8888", true},
			{"Invalid without port", "[2001:db8]", false},
			{"Invalid with port", "[2001:db8]:2222", false},
		}
		for _, subtest := range testData {
			st := subtest
			t.Run(st.name, func(t *testing.T) {
				require.Equal(t, st.isValid, ValidateAddress(st.address))
			})
		}
	})
}
