// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package enc

import (
	"crypto/aes"
	"crypto/cipher"
	"io"
	"io/ioutil"

	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// EeBuild indicates if this is a Enterprise build.
var EeBuild = true

const (
	encKeyFile = "encryption_key_file"
)

// RegisterFlags registers the required encryption flags.
func RegisterFlags(flag *pflag.FlagSet) {
	flag.String(encKeyFile, "",
		"The file that stores the symmetric key. The key size must be 16, 24, or 32 bytes long. "+
			"The key size determines the corresponding block size for AES encryption "+
			"(AES-128, AES-192, and AES-256 respectively). Enterprise feature.")

	// Register options for Vault stuff.
	registerVaultFlags(flag)
}

type KeyReader interface {
	ReadKey() (x.SensitiveByteSlice, error)
}

// localKeyReader implements the KeyReader interface. It reads the key from local files.
type localKeyReader struct {
	keyFile string
}

func (lkr *localKeyReader) ReadKey() (x.SensitiveByteSlice, error) {
	if lkr == nil {
		return nil, errors.Errorf("nil localKeyReader")
	}
	if lkr.keyFile == "" {
		return nil, errors.Errorf("bad localKeyReader")
	}
	k, err := ioutil.ReadFile(lkr.keyFile)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading encryption key file (%v)", lkr.keyFile)
	}
	// len must be 16,24,32 bytes if given. All other lengths are invalid.
	klen := len(k)
	if klen != 16 && klen != 24 && klen != 32 {
		return nil, errors.Errorf("invalid key length %d", klen)
	}
	return k, nil
}

// NewKeyReader returns a KeyReader interface based on the configuration options.
// Valid KeyReaders are:
// 1. Local to read key from local filesystem. .
// 2. Vault to read key from vault.
// 3. Nil when encryption is turned off.
func NewKeyReader(cfg *viper.Viper) (KeyReader, error) {
	var keyReaders int
	var keyReader KeyReader
	var err error

	keyFile := cfg.GetString(encKeyFile)
	roleID := cfg.GetString(vaultRoleIDFile)
	secretID := cfg.GetString(vaultSecretIDFile)

	if keyFile != "" {
		keyReader = &localKeyReader{
			keyFile: keyFile,
		}
		keyReaders++
	}
	if roleID != "" || secretID != "" {
		keyReader, err = newVaultKeyReader(cfg)
		if err != nil {
			return nil, err
		}
		keyReaders++
	}
	if keyReaders == 2 {
		return nil, errors.Errorf("cannot have local and vault key readers. " +
			"re-check the configuration")
	}
	glog.Infof("KeyReader instantiated of type %T", keyReader)
	return keyReader, nil
}

// GetWriter wraps a crypto StreamWriter using the input key on the input Writer.
func GetWriter(key x.SensitiveByteSlice, w io.Writer) (io.Writer, error) {
	// No encryption, return the input writer as is.
	if key == nil {
		return w, nil
	}
	// Encryption, wrap crypto StreamWriter on the input Writer.
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv, err := y.GenerateIV()
	if err != nil {
		return nil, err
	}
	if iv != nil {
		if _, err = w.Write(iv); err != nil {
			return nil, err
		}
	}
	return cipher.StreamWriter{S: cipher.NewCTR(c, iv), W: w}, nil
}

// GetReader wraps a crypto StreamReader using the input key on the input Reader.
func GetReader(key x.SensitiveByteSlice, r io.Reader) (io.Reader, error) {
	// No encryption, return input reader as is.
	if key == nil {
		return r, nil
	}

	// Encryption, wrap crypto StreamReader on input Reader.
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	var iv []byte = make([]byte, 16)
	cnt, err := r.Read(iv)
	if cnt != 16 || err != nil {
		err = errors.Errorf("unable to get IV from encrypted backup. Read %v bytes, err %v ",
			cnt, err)
		return nil, err
	}
	return cipher.StreamReader{S: cipher.NewCTR(c, iv), R: r}, nil
}

// ReadEncryptionKeyFile returns the encryption key in the given file.
func ReadEncryptionKeyFile(filepath string) x.SensitiveByteSlice {
	if filepath == "" {
		return nil
	}
	k, err := ioutil.ReadFile(filepath)
	x.Checkf(err, "Error reading encryption key file (%v)", filepath)

	// len must be 16,24,32 bytes if given. All other lengths are invalid.
	klen := len(k)
	x.AssertTruef(klen == 16 || klen == 24 || klen == 32,
		"Invalid encryption key length = %v", klen)

	return k
}
