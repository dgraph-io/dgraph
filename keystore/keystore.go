package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/ChainSafe/gossamer/crypto"
	"golang.org/x/crypto/blake2b"
)

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
func DecryptPrivateKey(data, password []byte) (crypto.PrivateKey, error) {
	pk, err := Decrypt(data, password)
	if err != nil {
		return nil, err
	}

	return crypto.DecodePrivateKey(pk)
}

// EncryptAndWriteToFile encrypts the `crypto.PrivateKey` using the password and saves it to the specificied file
func EncryptAndWriteToFile(filename string, pk crypto.PrivateKey, password []byte) error {
	data, err := EncryptPrivateKey(pk, password)
	if err != nil {
		return err
	}

	fp, err := filepath.Abs(filename)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(fp, data, 0644)
}

// ReadFromFileAndDecrypt reads ciphertext from a file and decrypts it using the password into a `crypto.PrivateKey`
func ReadFromFileAndDecrypt(filename string, password []byte) (pk crypto.PrivateKey, err error) {
	fp, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(filepath.Clean(fp))
	if err != nil {
		return nil, err
	}

	return DecryptPrivateKey(data, password)
}
