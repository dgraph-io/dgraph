package keyring

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

func TestNewKeyring(t *testing.T) {
	kr, err := NewKeyring()
	if err != nil {
		t.Fatal(err)
	}

	v := reflect.ValueOf(kr).Elem()
	for i := 0; i < v.NumField(); i++ {
		key := v.Field(i).Interface().(*sr25519.Keypair).Private().Hex()
		if key != privateKeys[i] {
			t.Fatalf("Fail: got %s expected %s", key, privateKeys[i])
		}
	}
}
