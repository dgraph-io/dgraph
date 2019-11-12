package crypto

type Keypair interface {
	Sign(msg []byte) []byte
	Public() PublicKey
	Private() PrivateKey
}

type PublicKey interface {
	Verify(msg, sig []byte) bool
	Encode() []byte
	Decode([]byte) error
}

type PrivateKey interface {
	Sign(msg []byte) []byte
	Public() PublicKey
	Encode() []byte
	Decode([]byte) error
}

func DecodePrivateKey(in []byte) (PrivateKey, error) {
	priv, err := NewEd25519PrivateKey(in)
	if err != nil {
		return nil, err
	}

	return priv, nil
}
