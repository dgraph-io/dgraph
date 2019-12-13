package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/ChainSafe/gossamer/crypto"
	"github.com/ChainSafe/gossamer/crypto/ed25519"
	"github.com/ChainSafe/gossamer/crypto/secp256k1"
	"github.com/ChainSafe/gossamer/crypto/sr25519"
	"golang.org/x/crypto/blake2b"
)

type EncryptedKeystore struct {
	Type       string
	PublicKey  string
	Ciphertext []byte
}

// gcmFromPassphrase creates a symmetric AES key given a password
func gcmFromPassphrase(password []byte) (cipher.AEAD, error) {
	hash := blake2b.Sum256(password)

	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return gcm, nil
}

// Encrypt uses AES to encrypt `msg` with the symmetric key deterministically created from `password`
func Encrypt(msg, password []byte) ([]byte, error) {
	gcm, err := gcmFromPassphrase(password)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, msg, nil)
	return ciphertext, nil
}

// Decrypt uses AES to decrypt ciphertext with the symmetric key deterministically created from `password`
func Decrypt(data, password []byte) ([]byte, error) {
	gcm, err := gcmFromPassphrase(password)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// EncryptPrivateKey uses AES to encrypt an encoded `crypto.PrivateKey` with a symmetric key deterministically
// created from `password`
func EncryptPrivateKey(pk crypto.PrivateKey, password []byte) ([]byte, error) {
	return Encrypt(pk.Encode(), password)
}

// DecryptPrivateKey uses AES to decrypt the ciphertext into a `crypto.PrivateKey` with a symmetric key deterministically
// created from `password`
func DecryptPrivateKey(data, password []byte, keytype string) (crypto.PrivateKey, error) {
	pk, err := Decrypt(data, password)
	if err != nil {
		return nil, err
	}

	return DecodePrivateKey(pk, keytype)
}

// EncryptAndWriteToFile encrypts the `crypto.PrivateKey` using the password and saves it to the specified file
func EncryptAndWriteToFile(file *os.File, pk crypto.PrivateKey, password []byte) error {
	ciphertext, err := EncryptPrivateKey(pk, password)
	if err != nil {
		return err
	}

	pub, err := pk.Public()
	if err != nil {
		return fmt.Errorf("cannot get public key: %s", err)
	}

	keytype := ""
	if _, ok := pk.(*ed25519.PrivateKey); ok {
		keytype = crypto.Ed25519Type
	}

	if _, ok := pk.(*sr25519.PrivateKey); ok {
		keytype = crypto.Sr25519Type
	}

	if _, ok := pk.(*secp256k1.PrivateKey); ok {
		keytype = crypto.Secp256k1Type
	}

	if keytype == "" {
		return errors.New("cannot write key not of type sr25519, ed25519, secp256k1")
	}

	keydata := &EncryptedKeystore{
		Type:       keytype,
		PublicKey:  pub.Hex(),
		Ciphertext: ciphertext,
	}

	data, err := json.MarshalIndent(keydata, "", "\t")
	if err != nil {
		return err
	}

	_, err = file.Write(append(data, byte('\n')))
	return err
}

// ReadFromFileAndDecrypt reads ciphertext from a file and decrypts it using the password into a `crypto.PrivateKey`
func ReadFromFileAndDecrypt(filename string, password []byte) (crypto.PrivateKey, error) {
	fp, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(filepath.Clean(fp))
	if err != nil {
		return nil, err
	}

	keydata := new(EncryptedKeystore)
	err = json.Unmarshal(data, keydata)
	if err != nil {
		return nil, err
	}

	return DecryptPrivateKey(keydata.Ciphertext, password, keydata.Type)
}
