/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

var expiredKey = []byte(`-----BEGIN PGP MESSAGE-----

owEBwgI9/ZANAwAKAXomeeH3SyppAax7YgxsaWNlbnNlLmpzb25etF5+ewogICJ1
c2VyIjogIkRncmFwaCBUZXN0IEtleSIsCiAgIm1heF9ub2RlcyI6IDE4NDQ2NzQ0
MDczNzA5NTUxNjE1LAogICJleHBpcnkiOiAiMTk3MC0wMS0wMVQwMDowMDowMFoi
Cn0KiQIzBAABCgAdFiEED3lYS97wtaMT1MW+eiZ54fdLKmkFAl60Xn4ACgkQeiZ5
4fdLKmlVYQ//afX0H7Seb0ukxCMAcM8uqlBEGCEFv3K34avk1g8XRa6y4q/Ys0uU
DSaaDWdQ8IS5Q9SNlZBbJuqO6Pf1R01dEPTYQizWkDjYIBsY9xJnMZKEaA+F3bkn
8TXqI588+AvbqxHosz8cvh/nG+Ajk451rI9c2bqKB/FvH/zI6XyfUjqN+PvrqH0E
POA7nqSrWDemW4cMgNR4PhXehB/n2i3G6cPpwgCUd+N00N1f1mir/LmL6G5T4PrG
BmVz9fOdEr+U85PbMF9vOke9LkLQYdnF1hEV+7++t2/uoaLDYbxYhUnXpJZxwCBX
DQTievpyQF47HzuifvqUyxDSEsYiSGhhap1e/tvf1VaZoFUuTYQQpiV7+9K3UrL0
SnJ5TRWS7cEKBLyZszrakGpqVakvEUlWO4wG0Fp4kUa4YXw8t58oqHRn9aAtoqJd
UOLnq2semUttaySR4DHhjneO3/RoVm79/aaqMi/QNJzc9Tt9nY0AgcYlA3bVXmAZ
nM9Rgi6SaO5DxnRdhFzZyYQMb4onFDI0eYMOhPm+NmKWplkFXB+mKPKj5o/pcEb4
SWHt8fUAWDLsmcooIixDmSay14aBmF08hQ1vtJkY7/jo3hlK36GrLnNdN4IODqk/
I8mUd/jcj3NZtGWFoxKq4laK/ruoeoHnWMznJyMm75nzcU5QZU9yEEI=
=2lFm
-----END PGP MESSAGE-----
`)

var invalidKey = []byte(`-----BEGIN PGP MESSAGE-----

x7YgxsaWNlbnNlLmpzb25etF5owEBwgI9omeeH3SyppAa/ZANAwAKAX+ewogICJ1
c2VyIjogIkRncmFwaCBUZXN0IEtleSIsCiAgIm1heF9ub2RlcyI6IDE4NDQ2NzQ0
MDczNzA5NTUxNjE1LAogICJleHBpcnkiOiAiMTk3MC0wMS0wMVQwMDowMDowMFoi
Cn0KiQIzBAABCgAdFiEED3lYS97wtaMT1MW+eiZ54fdLKmkFAl60Xn4ACgkQeiZ5
4fdLKmlVYQ//afX0H7Seb0ukxCMAcM8uqlBEGCEFv3K34avk1g8XRa6y4q/Ys0uU
DSaaDWdQ8QizWkDjYIBsY9xJnMZKEaAIS5Q9SNlZBbJuqO6Pf1R01dEPTY+F3bkn
8T1rI9c2bqKB/FvH/zI6XXqI588+AvbqxHosz8cvh/nG+Ajk45yfUjqN+PvrqH0E
POA7nqSrWDemW4cMgNR4PhXehB/n2i3G6cPpwgCUd+N00N1f1mir/LmL6G5T4PrG
BmVz9fOdEr+U85PbMF9vOke9LkLQYdnF1hEV+7++t2/uoaLDYbxYhUnXpJZxwCBX
DQTievpyQxDSEsYiSGhhap1e/tvf1VaZoFUuTYQQpiV7F47HzuifvqUy+9K3UrL0
SnJ5TRWS7cEKBLyZszrakGpqVakvEUlWO4wG0Fp4kUa4YXw8t58oqHRn9aAtoqJd
UOLnq2semUttaySR4DHhjneO3/RoVm79/aaqMi/QNJzc9Tt9nY0AgcYlA3bVXmAZ
nM9Rgi6SaO5DxnRdhFzZyYQMb4onFDI0eYMOhPm+NmKWplkFXB+mKPKj5o/pcEb4
SWHt8fUAWDLsmcJkY7/jo3hlK36GrLnNdN4IODqkooIixDmSay14aBmF08hQ1vt/
I8jcj3NZtGWFoxKq4laK/ruoeoHnWMznJyMm7mUd/5nzcU5QZU9yEEI=
=2lFm
-----END PGP MESSAGE-----
`)

type Location struct {
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
}

type GqlError struct {
	Message    string                 `json:"message"`
	Locations  []Location             `json:"locations,omitempty"`
	Path       []interface{}          `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

type GqlErrorList []*GqlError

type responseStruct struct {
	Errors  GqlErrorList           `json:"errors"`
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	License map[string]interface{} `json:"license"`
}

func TestEnterpriseLicense(t *testing.T) {

	stateURL := "http://localhost:6180/state"
	enterpriseLicenseURL := "http://localhost:6180/enterpriseLicense"

	var tests = []struct {
		name       string
		licenseKey []byte
		code       string
		user       string
		message    string
	}{
		{
			"Using expired entrerprise license key, should be able to extract user information",
			expiredKey,
			`Success`,
			`Dgraph Test Key`,
			``,
		},
		{
			"Using invalid entrerprise license key should return an error",
			invalidKey,
			``,
			``,
			`while extracting enterprise details from the license: while reading PGP message from license file: openpgp: unsupported feature: public key version`,
		},
		{
			"Using empty entrerprise license key should return an error",
			[]byte(``),
			``,
			``,
			`while extracting enterprise details from the license: while decoding license file: EOF`,
		},
	}
	for _, tt := range tests {

		// Apply the license
		response, err := http.Post(enterpriseLicenseURL, "application/text", bytes.NewBuffer(tt.licenseKey))
		require.NoError(t, err)

		var enterpriseResponse responseStruct
		responseBody, err := ioutil.ReadAll(response.Body)
		require.NoError(t, err)
		err = json.Unmarshal(responseBody, &enterpriseResponse)
		require.NoError(t, err)

		// Check if the license is applied
		require.Equal(t, enterpriseResponse.Code, tt.code)

		if enterpriseResponse.Code == `Success` {

			// check the user information in case the license is applied
			// Expired license should not be enabled even after it is applied

			response, err := http.Get(stateURL)
			require.NoError(t, err)

			var stateResponse responseStruct
			responseBody, err := ioutil.ReadAll(response.Body)
			require.NoError(t, err)
			err = json.Unmarshal(responseBody, &stateResponse)
			require.NoError(t, err)

			require.Equal(t, stateResponse.License["user"], tt.user)
			require.Equal(t, stateResponse.License["enabled"], false)
		} else {
			// check the error message in case the license is not applied
			require.Equal(t, enterpriseResponse.Errors[0].Message, tt.message)
		}
	}
}
