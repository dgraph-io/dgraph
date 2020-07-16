package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/dgraph-io/dgraph/testutil"
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

type responseStruct struct {
	Errors  json.RawMessage        `json:"errors"`
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	License map[string]interface{} `json:"license"`
}

func TestEnterpriseLicense(t *testing.T) {

	stateURL := "http://" + testutil.SockAddrZeroHttp + "/state"
	enterpriseLicenseURL := "http://" + testutil.SockAddrZeroHttp + "/enterpriseLicense"

	var tests = []struct {
		name           string
		licenseKey     []byte
		expectError    bool
		code           string
		user           string
		licenseEnabled interface{}
	}{
		{
			"Using expired entrerprise license key, should be able to extract user information",
			expiredKey,
			false,
			`Success`,
			`Dgraph Test Key`,
			nil,
		},
		{
			"Using invalid entrerprise license key should return an error",
			invalidKey,
			false,
			``,
			``,
			nil,
		},
		{
			"Using empty entrerprise license key should return an error",
			[]byte(``),
			false,
			``,
			``,
			nil,
		},
	}
	for _, tt := range tests {

		// Apply the license
		response, err := http.Post(enterpriseLicenseURL, "application/text", bytes.NewBuffer(tt.licenseKey))
		require.NoError(t, err)

		var enterpriseResponse responseStruct
		responseBody, err := ioutil.ReadAll(response.Body)
		require.NoError(t, err)
		json.Unmarshal(responseBody, &enterpriseResponse)

		// Check if the license is applied
		require.Equal(t, enterpriseResponse.Code, tt.code)

		// Expired license should not be enabled even after it is applied
		if enterpriseResponse.Code == `Success` {

			response, err := http.Get(stateURL)
			require.NoError(t, err)

			var stateResponse responseStruct
			responseBody, err := ioutil.ReadAll(response.Body)
			require.NoError(t, err)
			json.Unmarshal(responseBody, &stateResponse)

			require.Equal(t, stateResponse.License["user"], tt.user)
			require.Equal(t, stateResponse.License["enabled"], tt.licenseEnabled)
		}
	}
}
