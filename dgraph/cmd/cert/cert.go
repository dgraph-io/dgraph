/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"
)

// makeKey generates an RSA private key of bitSize length, storing it in the
// file fn. If force is true, the file is replaced.
// Returns the RSA private key, or error otherwise.
func makeKey(fn string, bitSize int, force bool) (*rsa.PrivateKey, error) {
	if _, err := os.Stat(fn); os.IsExist(err) && !force {
		return readKey(fn)
	}

	key, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		return nil, err
	}

	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !force {
		flag |= os.O_EXCL
	}

	f, err := os.OpenFile(fn, flag, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	err = pem.Encode(f, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if err != nil {
		return nil, err
	}

	return key, nil
}

// readKey tries to read and decode the contents of a private key at fn.
// Returns the RSA private key, or error otherwise.
func readKey(fn string) (*rsa.PrivateKey, error) {
	b, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(b)
	switch {
	case block == nil:
		return nil, fmt.Errorf("failed to read key block")
	case block.Type != "RSA PRIVATE KEY":
		return nil, fmt.Errorf("unknown PEM type: %s", block.Type)
	}

	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// readCert tries to read and decode the contents of an RSA-signed cert at fn.
// Returns the x509v3 cert, or error otherwise.
func readCert(fn string) (*x509.Certificate, error) {
	b, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(b)
	switch {
	case block == nil:
		return nil, fmt.Errorf("failed to read cert block")
	case block.Type != "CERTIFICATE":
		return nil, fmt.Errorf("unknown PEM type: %s", block.Type)
	}

	return x509.ParseCertificate(block.Bytes)
}

// createCAPair creates a CA certificate and key pair. The key file is created only
// if it doesn't already exist or we force it. The key path can differ from the certsDir
// which case the path must already exist and be writable.
// Returns nil on success, or an error otherwise.
func createCAPair(certsDir, keyPath, certFile string, keySize int, d time.Duration, force bool) error {
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return err
	}

	// no path then save it in certsDir.
	if path.Base(keyPath) == keyPath {
		keyPath = filepath.Join(certsDir, keyPath)
	}

	r, err := NewRequest(
		cfCA(true),
		cfDuration(d),
		cfKeySize(keySize),
		cfOverwrite(force),
	)
	if err != nil {
		return err
	}

	return r.GeneratePair(
		keyPath,
		filepath.Join(certsDir, certFile),
	)
}

// createNodePair creates a node certificate and key pair. The key file is created only
// if it doesn't already exist or we force it. The key path can differ from the certsDir
// which case the path must already exist and be writable.
// Returns nil on success, or an error otherwise.
func createNodePair(certsDir, keyPath, certFile string, keySize int, d time.Duration, force bool, nodes []string) error {
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return err
	}

	// no path then save it in certsDir.
	if path.Base(keyPath) == keyPath {
		keyPath = filepath.Join(certsDir, keyPath)
	}

	r, err := NewRequest(
		cfParent(filepath.Join(certsDir, defaultCACert)),
		cfSignKey(keyPath),
		cfDuration(d),
		cfKeySize(keySize),
		cfOverwrite(force),
		cfHosts(nodes...),
	)
	if err != nil {
		return err
	}

	return r.GeneratePair(
		filepath.Join(certsDir, defaultNodeKey),
		filepath.Join(certsDir, defaultNodeCert),
	)
}
