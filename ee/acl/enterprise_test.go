package acl

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"
)

func TestEnterpriseLicense(t *testing.T) {

	expiredKey := []byte(`-----BEGIN PGP MESSAGE-----
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
	enterpriseLicenseURL := "http://localhost:6080/enterpriseLicense"
	resp, _ := http.Post(enterpriseLicenseURL, "application/text", bytes.NewBuffer(expiredKey))
	data, _ := ioutil.ReadAll(resp.Body)
	var finalData interface{}
	json.Unmarshal(data, &finalData)
	xx := finalData.(map[string]interface{})["errors"]
	t.Log("++++ errors +++\n", xx)
	t.Log("woohoo\n", (xx.([]interface{}))[0].(map[string]interface{})["message"])
	t.Log(reflect.TypeOf(xx))
	// for i := range finalData {
	// 	t.Log("testsetet\n", i, finalData[i])
	// }
	// vall := RetrieveDeepMap([]string{"errors", "message"}, finalData)
	// t.Log("finalvalue", vall)

	// enterpriseLicenseURL := "http://localhost:6080/enterpriseLicense"

	// var tests = []struct {
	// 	name           string
	// 	licenseKey     []byte
	// 	expectError    bool
	// 	expectedOutput string
	// }{
	// 	{
	// 		"Using expired entrerprised license key should return an error",
	// 		expiredKey,
	// 		false,
	// 		`{"errors":[{"message":"while extracting enterprise details from the license: while decoding license file: EOF","extensions":{"code":"ErrorInvalidRequest"}}]}`,
	// 	},
	// }
	// for _, tt := range tests {
	// 	t.Logf("Running: %s\n", tt.name)
	// 	response, err := http.Post(enterpriseLicenseURL, "application/text", bytes.NewBuffer(expiredKey))
	// 	returnedOutput, err := ioutil.ReadAll(response.Body)
	// 	if tt.expectError {
	// 		require.Error(t, err)
	// 		continue
	// 	}

	// 	require.NoError(t, err)
	// 	require.Equal(t, tt.expectedOutput, string(returnedOutput))
	// }

}
