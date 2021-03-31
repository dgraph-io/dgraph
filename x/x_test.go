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
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSensitiveByteSlice(t *testing.T) {
	var v Sensitive = Sensitive("mysecretkey")

	s := fmt.Sprintf("%s,%v,%s,%+v", v, v, &v, &v)
	require.EqualValues(t, "****,****,****,****", s)
}

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
			err     string
		}{
			{"Valid without port", "190.0.0.1", "address 190.0.0.1: missing port in address"},
			{"Valid with port", "192.5.32.1:333", ""},
			{"Invalid without port", "12.0.0", "address 12.0.0: missing port in address"},
			// the following test returns true because 12.0.0 is considered as valid
			// hostname
			{"Valid with port", "12.0.0:3333", ""},
			{"Invalid port", "190.0.0.1:222222", "Invalid port: 222222"},
		}
		for _, st := range testData {
			t.Run(st.name, func(t *testing.T) {
				if st.err != "" {
					require.EqualError(t, ValidateAddress(st.address), st.err)
				} else {
					require.NoError(t, ValidateAddress(st.address))
				}
			})
		}

	})
	t.Run("IPv6", func(t *testing.T) {
		testData := []struct {
			name    string
			address string
			err     string
		}{
			{"Valid without port", "[2001:db8::1]", "address [2001:db8::1]: missing port in address"},
			{"Valid with port", "[2001:db8::1]:8888", ""},
			{"Invalid without port", "[2001:db8]", "address [2001:db8]: missing port in address"},
			{"Invalid with port", "[2001:db8]:2222", "Invalid hostname: 2001:db8"},
			{"Invalid port", "[2001:db8::1]:222222", "Invalid port: 222222"},
		}
		for _, st := range testData {
			t.Run(st.name, func(t *testing.T) {
				if st.err != "" {
					require.EqualError(t, ValidateAddress(st.address), st.err)
				} else {
					require.NoError(t, ValidateAddress(st.address))
				}
			})
		}
	})
	t.Run("Hostnames", func(t *testing.T) {
		testData := []struct {
			name    string
			address string
			err     string
		}{
			{"Valid", "dgraph-alpha-0.dgraph-alpha-headless.default.svc.local:9080", ""},
			{"Valid with underscores", "alpha_1:9080", ""},
			{"Valid ending in a period", "dgraph-alpha-0.dgraph-alpha-headless.default.svc.:9080", ""},
			{"Invalid because the name part is longer than 63 characters",
				"this-is-a-name-that-is-way-too-long-for-a-hostname-that-is-valid:9080",
				"Invalid hostname: " +
					"this-is-a-name-that-is-way-too-long-for-a-hostname-that-is-valid"},
			{"Invalid because it starts with a hyphen", "-alpha1:9080", "Invalid hostname: -alpha1"},
		}
		for _, st := range testData {
			t.Run(st.name, func(t *testing.T) {
				if st.err != "" {
					require.EqualError(t, ValidateAddress(st.address), st.err)
				} else {
					require.NoError(t, ValidateAddress(st.address))
				}
			})
		}

	})
}

func TestGqlError(t *testing.T) {
	tests := map[string]struct {
		err error
		req string
	}{
		"GqlError": {
			err: GqlErrorf("A GraphQL error"),
			req: "A GraphQL error",
		},
		"GqlError with a location": {
			err: GqlErrorf("A GraphQL error").WithLocations(Location{Line: 1, Column: 8}),
			req: "A GraphQL error (Locations: [{Line: 1, Column: 8}])",
		},
		"GqlError with many locations": {
			err: GqlErrorf("A GraphQL error").
				WithLocations(Location{Line: 1, Column: 2}, Location{Line: 1, Column: 8}),
			req: "A GraphQL error (Locations: [{Line: 1, Column: 2}, {Line: 1, Column: 8}])",
		},
		"GqlErrorList": {
			err: GqlErrorList{GqlErrorf("A GraphQL error"), GqlErrorf("Another GraphQL error")},
			req: "A GraphQL error\nAnother GraphQL error",
		},
	}

	for name, tcase := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tcase.req, tcase.err.Error())
		})
	}
}

func TestVersionString(t *testing.T) {
	dgraphVersion = "v1.2.2-rc1-g1234567"
	require.True(t, DevVersion())

	dgraphVersion = "v20.03-1-beta-Mar20-g12345678"
	require.True(t, DevVersion())

	dgraphVersion = "v20.03"
	require.False(t, DevVersion())

	// less than 7 hex digits in commit-hash
	dgraphVersion = "v1.2.2-rc1-g123456"
	require.False(t, DevVersion())

}

func TestToHex(t *testing.T) {
	require.Equal(t, []byte(`"0x0"`), ToHex(0, false))
	require.Equal(t, []byte(`<0x0>`), ToHex(0, true))
	require.Equal(t, []byte(`"0xf"`), ToHex(15, false))
	require.Equal(t, []byte(`<0xf>`), ToHex(15, true))
	require.Equal(t, []byte(`"0x19"`), ToHex(25, false))
	require.Equal(t, []byte(`<0x19>`), ToHex(25, true))
	require.Equal(t, []byte(`"0xff"`), ToHex(255, false))
	require.Equal(t, []byte(`<0xff>`), ToHex(255, true))
	require.Equal(t, []byte(`"0xffffffffffffffff"`), ToHex(math.MaxUint64, false))
	require.Equal(t, []byte(`<0xffffffffffffffff>`), ToHex(math.MaxUint64, true))
}
