package crypto_test

import (
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"

	"github.com/stretchr/testify/require"
)

func TestPubkeyToAddress(t *testing.T) {
	// randomly generated from subkey
	pub, _ := common.HexToBytes("0x6ec2950d29adda8d965d06fc78b7e05f8923b8de3e312c7b5957cdcfd8d4820c")
	addr := "5EZvvkH5RUjigUNT7pabMzMnHtmrYamsSe7yW6vVACBzTHFe"

	pk, err := sr25519.NewPublicKey(pub)
	require.NoError(t, err)
	a := pk.Address()
	require.Equal(t, addr, string(a))
}
