/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package cert

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

const (
	defaultDir      = "tls"
	defaultDays     = 1826
	defaultCADays   = 3651
	defaultCACert   = "ca.crt"
	defaultCAKey    = "ca.key"
	defaultKeySize  = 2048
	defaultNodeCert = "node.crt"
	defaultNodeKey  = "node.key"
	keySizeTooSmall = 512
	keySizeTooLarge = 4096
)

// makeKey generates an RSA or ECDSA private key using the configuration in 'c'.
// The new private key is stored in the path at 'keyFile'.
// If force is true, any existing file at the path is replaced.
// For RSA, the configuration keySize is used for length.
// For ECDSA, the configuration elliptical curve is used.
// Returns the RSA or ECDSA private key, or error otherwise.
func makeKey(keyFile string, c *certConfig) (crypto.PrivateKey, error) {
	fp, err := safeCreate(keyFile, c.force, 0600)
	if err != nil {
		// reuse the existing key, if possible.
		if os.IsExist(err) {
			return readKey(keyFile)
		}
		return nil, err
	}
	defer fp.Close()

	var key crypto.PrivateKey
	switch c.curve {
	case "":
		key, err = rsa.GenerateKey(rand.Reader, c.keySize)
	case "P224":
		key, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case "P256":
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "P384":
		key, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "P521":
		key, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	}
	if err != nil {
		return nil, err
	}

	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return key, pem.Encode(fp, &pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: b,
		})
	case *rsa.PrivateKey:
		return key, pem.Encode(fp, &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		})
	}
	return nil, errors.Errorf("Unsupported key type: %T", key)
}

// readKey tries to read and decode the contents of a private key file.
// Returns the private key, or error otherwise.
func readKey(keyFile string) (crypto.PrivateKey, error) {
	b, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(b)
	switch {
	case block == nil:
		return nil, fmt.Errorf("Failed to read key block")
	case block.Type == "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case block.Type == "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	return nil, fmt.Errorf("Unknown PEM type: %s", block.Type)
}

// readCert tries to read and decode the contents of a signed cert file.
// Returns the x509v3 cert, or error otherwise.
func readCert(certFile string) (*x509.Certificate, error) {
	b, err := ioutil.ReadFile(certFile)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(b)
	switch {
	case block == nil:
		return nil, fmt.Errorf("Failed to read cert block")
	case block.Type != "CERTIFICATE":
		return nil, fmt.Errorf("Unknown PEM type: %s", block.Type)
	}

	return x509.ParseCertificate(block.Bytes)
}

// safeCreate only creates a file if it doesn't exist or we force overwrite.
func safeCreate(name string, overwrite bool, perm os.FileMode) (*os.File, error) {
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flag |= os.O_EXCL
	}
	return os.OpenFile(name, flag, perm)
}

// createCAPair creates a CA certificate and key pair. The key file is created only
// if it doesn't already exist or we force it. The key path can differ from the certsDir
// which case the path must already exist and be writable.
// Returns nil on success, or an error otherwise.
func createCAPair(opt options) error {
	cc := certConfig{
		isCA:    true,
		until:   defaultCADays,
		keySize: opt.keySize,
		force:   opt.force,
		curve:   opt.curve,
	}
	if err := cc.generatePair(opt.caKey, opt.caCert); err != nil {
		return err
	}

	// secure CA key
	return os.Chmod(opt.caKey, 0400)
}

// createNodePair creates a node certificate and key pair. The key file is created only
// if it doesn't already exist or we force it. The key path can differ from the certsDir
// which case the path must already exist and be writable.
// Returns nil on success, or an error otherwise.
func createNodePair(opt options) error {
	if opt.nodes == nil || len(opt.nodes) == 0 {
		return nil
	}

	cc := certConfig{
		until:   opt.days,
		keySize: opt.keySize,
		force:   opt.force,
		hosts:   opt.nodes,
		curve:   opt.curve,
	}

	var err error
	cc.parent, err = readCert(opt.caCert)
	if err != nil {
		return err
	}
	{
		priv, err := readKey(opt.caKey)
		if err != nil {
			return err
		}
		cc.signer = priv.(crypto.Signer)
	}

	certFile := filepath.Join(opt.dir, defaultNodeCert)
	keyFile := filepath.Join(opt.dir, defaultNodeKey)
	err = cc.generatePair(keyFile, certFile)
	if err != nil || !opt.verify {
		return err
	}

	return cc.verifyCert(certFile)
}

// createClientPair creates a client certificate and key pair. The key file is created only
// if it doesn't already exist or we force it. The key path can differ from the certsDir
// which case the path must already exist and be writable.
// Returns nil on success, or an error otherwise.
func createClientPair(opt options) error {
	if opt.client == "" {
		return nil
	}

	cc := certConfig{
		until:   opt.days,
		keySize: opt.keySize,
		force:   opt.force,
		client:  opt.client,
		curve:   opt.curve,
	}

	var err error
	cc.parent, err = readCert(opt.caCert)
	if err != nil {
		return err
	}
	{
		priv, err := readKey(opt.caKey)
		if err != nil {
			return err
		}
		cc.signer = priv.(crypto.Signer)
	}

	certFile := filepath.Join(opt.dir, fmt.Sprint("client.", opt.client, ".crt"))
	keyFile := filepath.Join(opt.dir, fmt.Sprint("client.", opt.client, ".key"))
	err = cc.generatePair(keyFile, certFile)
	if err != nil || !opt.verify {
		return err
	}

	return cc.verifyCert(certFile)
}

func createCerts(opt options) error {
	if opt.dir == "" {
		return errors.New("Invalid TLS directory")
	}

	err := os.Mkdir(opt.dir, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}

	switch {
	case opt.keySize < keySizeTooSmall:
		return errors.New("Key size value is too small (x < 512)")
	case opt.keySize > keySizeTooLarge:
		return errors.New("Key size value is too large (x > 4096)")
	case opt.keySize%2 != 0:
		return errors.New("Key size value must be a factor of 2")
	}

	switch opt.curve {
	case "":
	case "P224", "P256", "P384", "P521":
	default:
		return errors.New(`Elliptic curve value must be one of: P224, P256, P384 or P521`)
	}

	// no path then save it in certsDir.
	if path.Base(opt.caKey) == opt.caKey {
		opt.caKey = filepath.Join(opt.dir, opt.caKey)
	}
	opt.caCert = filepath.Join(opt.dir, defaultCACert)

	if err := createCAPair(opt); err != nil {
		return err
	}
	if err := createNodePair(opt); err != nil {
		return err
	}
	return createClientPair(opt)
}
