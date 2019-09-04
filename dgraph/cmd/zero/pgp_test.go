package zero

import (
	"bytes"
	"crypto"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/packet"
)

func encodePublicKey(t *testing.T, e *openpgp.Entity) *bytes.Buffer {
	b := new(bytes.Buffer)
	encodedKeyBuf, err := armor.Encode(b, openpgp.PublicKeyType, nil)
	require.NoError(t, err)
	err = e.Serialize(encodedKeyBuf)
	require.NoError(t, err)
	err = encodedKeyBuf.Close()
	require.NoError(t, err)
	return b
}

func signAndWriteMessage(t *testing.T, entity *openpgp.Entity, json string) *bytes.Buffer {
	b := new(bytes.Buffer)
	w, err := openpgp.Sign(b, entity, nil, &packet.Config{
		RSABits:     4096,
		DefaultHash: crypto.SHA512,
	})
	require.NoError(t, err)

	_, err = w.Write([]byte(json))
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	// armor encode the message
	abuf := new(bytes.Buffer)
	w, err = armor.Encode(abuf, "PGP MESSAGE", nil)
	require.NoError(t, err)
	_, err = w.Write(b.Bytes())
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	return abuf
}

func TestEnterpriseDetails(t *testing.T) {
	correctEntity, err := openpgp.NewEntity("correct", "", "correct@correct.com", &packet.Config{
		RSABits:     4096,
		DefaultHash: crypto.SHA512,
	})

	require.NoError(t, err)
	incorrectEntity, err := openpgp.NewEntity("incorrect", "", "incorrect@incorrect.com", &packet.Config{
		RSABits:     4096,
		DefaultHash: crypto.SHA512,
	})
	require.NoError(t, err)
	correctJSON := `{"user": "user", "max_nodes": 10, "expiry": "2019-08-16T19:09:06+10:00"}`
	correctTime, err := time.Parse(time.RFC3339, "2019-08-16T19:09:06+10:00")
	require.NoError(t, err)

	var tests = []struct {
		name            string
		signingEntity   *openpgp.Entity
		json            string
		verifyingEntity *openpgp.Entity
		expectError     bool
		expectedOutput  license
	}{
		{
			"Signing JSON with empty data should return an error",
			correctEntity,
			`{}`,
			correctEntity,
			true,
			license{},
		},
		{
			"Signing JSON with incorrect private key should return an error",
			incorrectEntity,
			correctJSON,
			correctEntity,
			true,
			license{},
		},
		{
			"Verifying data with incorrect public key should return an error",
			correctEntity,
			correctJSON,
			incorrectEntity,
			true,
			license{},
		},
		{
			"Verifying data with correct public key should return correct data",
			correctEntity,
			correctJSON,
			correctEntity,
			false,
			license{"user", 10, correctTime},
		},
	}

	for _, tt := range tests {
		t.Logf("Running: %s\n", tt.name)
		buf := signAndWriteMessage(t, tt.signingEntity, tt.json)
		e := license{}
		publicKey := encodePublicKey(t, tt.verifyingEntity)
		err = verifySignature(buf, publicKey, &e)
		if tt.expectError {
			require.Error(t, err)
			continue
		}

		require.NoError(t, err)
		require.Equal(t, tt.expectedOutput, e)
	}
}
