package zero

import (
	"bytes"
	"crypto"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/packet"
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
	correctEntity, _ := openpgp.NewEntity("correct", "", "correct@correct.com", &packet.Config{
		RSABits:     4096,
		DefaultHash: crypto.SHA512,
	})
	correctJSON := `{"user": "user", "max_nodes": 10, "expiry": "2021-08-16T19:09:06+10:00"}`
	buf := signAndWriteMessage(t, correctEntity, correctJSON)
	publicKey = fmt.Sprintf("%v", correctEntity.PrimaryKey.PublicKey)
	publicKeyLocal := encodePublicKey(t, correctEntity)
	e := license{}
	t.Log(buf)
	_ = verifySignature(buf, publicKeyLocal, &e)
	t.Log(buf)
	response, _ := http.Post(enterpriseLicenseURL, "application/text", bytes.NewBuffer(buf.Bytes()))
	returnedOutput1, _ := ioutil.ReadAll(response.Body)
	t.Log(string(returnedOutput1))

	var tests = []struct {
		name           string
		licenseKey     []byte
		expectError    bool
		expectedOutput string
	}{
		{
			"Using expired entrerprised license key should return an error",
			expiredKey,
			false,
			`while extracting enterprise details from the license: while decoding license file: EOF`,
		},
	}
	for _, tt := range tests {
		t.Logf("Running: %s\n", tt.name)
		response, err := http.Post(enterpriseLicenseURL, "application/text", bytes.NewBuffer(expiredKey))
		returnedOutput, err := ioutil.ReadAll(response.Body)
		var finalData interface{}
		t.Log(response)
		json.Unmarshal(returnedOutput, &finalData)
		t.Log("\nstatus\n", finalData)
		errors := finalData.(map[string]interface{})["errors"].([]interface{})[0].(map[string]interface{})["message"]
		if tt.expectError {
			require.Error(t, err)
			continue
		}

		require.NoError(t, err)
		require.Equal(t, tt.expectedOutput, errors)
	}

}
