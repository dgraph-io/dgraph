/*
 * Copyright 2017-2021 Dgraph Labs, Inc. and Contributors
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

package audit

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

var key = []byte("12345678901234561234567890123456")
var entry = []byte("abcdefghijklmnopqrstuvwxyz")

func TestEncryptionCBC(t *testing.T) {
	file, err := os.OpenFile("test_encryption_chunks_cbc.txt", os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0600)
	require.Nil(t, err)
	salt := make([]byte, 8) // Generate an 8 byte salt
	rand.Read(salt)
	data := make([]byte, len(entry) + aes.BlockSize)
	copy(data[0:], "Salted__")
	copy(data[8:], salt)

	for i := 0; i<100; i ++ {
		copy(data[aes.BlockSize:], entry)
		padded, err := pkcs7Pad(data, aes.BlockSize)
		c, err := aes.NewCipher(key)
		require.Nil(t, err)
		iv := make([]byte, aes.BlockSize)
		rand.Read(iv)
		cbc := cipher.NewCBCEncrypter(c, iv)
		cbc.CryptBlocks(padded[aes.BlockSize:], padded[aes.BlockSize:])
		file.Write(padded)
	}

	file.Close()
}

// pkcs7Pad appends padding.
func pkcs7Pad(data []byte, blocklen int) ([]byte, error) {
	if blocklen <= 0 {
		return nil, fmt.Errorf("invalid blocklen %d", blocklen)
	}
	padlen := 1
	for ((len(data) + padlen) % blocklen) != 0 {
		padlen++
	}

	pad := bytes.Repeat([]byte{byte(padlen)}, padlen)
	return append(data, pad...), nil
}

func TestEncryptionInChunks(t *testing.T) {
	file, err := os.OpenFile("test_encryption_chunks.txt", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	require.Nil(t, err)
	iv := make([]byte, aes.BlockSize)
	rand.Read(iv)

	block, err := aes.NewCipher(key)
	require.Nil(t, err)

	stream := cipher.NewCTR(block, iv)
	for i:=0; i< 100; i++ {
		e := []byte("abcdefghijklmnopqrstuvwxyz")
		stream.XORKeyStream(e, e)
		file.Write(e)
	}
	file.Write(iv)
}

func TestDecryptionInChunks(t *testing.T) {
	cipherText, err := os.Open("test_encryption_chunks.txt")
	require.Nil(t, err)
	iv := make([]byte, aes.BlockSize)
	stat, _ := cipherText.Stat()
	msg := stat.Size() - int64(len(iv))
	_, err = cipherText.ReadAt(iv, msg)
	require.Nil(t, err)
	outfile, err := os.OpenFile("test_encryption_chunks_out.txt",
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0600)
	require.Nil(t, err)

	block, err := aes.NewCipher(key)
	require.Nil(t, err)

	buf := make([]byte, 26)
	stream := cipher.NewCTR(block, iv)

	for {
		n, err := cipherText.Read(buf)
		if n > 0 {
			if msg == 0 {
				break
			}
			msg = msg - int64(n)
			stream.XORKeyStream(buf, buf)
			outfile.Write(buf)
		}

		if err == io.EOF { break }
		require.Nil(t, err)
	}
}

func TestEncryptionAtOnce(t *testing.T) {
	file, err := os.OpenFile("test_encryption_once.txt", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	require.Nil(t, err)
	block, err := aes.NewCipher(key)
	require.Nil(t, err)
	iv := make([]byte, aes.BlockSize)
	rand.Read(iv)
	gcmCipher := cipher.NewCBCEncrypter(block, iv)
	repeat := bytes.Repeat(entry, 100)

	gcmCipher.CryptBlocks(repeat, repeat)
	file.Write(repeat)
}

func TestDecryptionAtOnce(t *testing.T) {
	cipherText, err := ioutil.ReadFile("test_encryption_once.txt")
	require.Nil(t, err)
	block, err := aes.NewCipher(key)
	require.Nil(t, err)
	gcm, err := cipher.NewGCM(block)
	require.Nil(t, err)
	nonce := cipherText[:gcm.NonceSize()]
	cipherText = cipherText[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, cipherText, nil)
	require.Nil(t, err)
	fmt.Println(string(plaintext))
}