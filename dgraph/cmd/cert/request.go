/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cert

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math"
	"math/big"
	"net"
	"os"
	"time"
)

const (
	dnOrganization  = "Dgraph"
	keySizeTooSmall = 512 // XXX: Microsoft is using 1024
	keySizeTooLarge = 4096
	validNotBefore  = time.Hour * -24
)

type config struct {
	parent  *x509.Certificate
	sign    crypto.Signer
	until   time.Duration
	ca      bool
	keySize int
	force   bool
	hosts   []string
	user    string
}

type configFunc func(*config) error

// Request describes a cert configuration.
type Request struct {
	c config
}

// cfParent sets a parent cert (e.g., CA)
// Returns an error if it cant read the parent cert file.
func cfParent(s string) configFunc {
	return func(c *config) error {
		parent, err := readCert(s)
		if err != nil {
			return err
		}
		c.parent = parent
		return nil
	}
}

// cfSignKey sets the signing private key
// Returns an error if it cant read the private key file.
func cfSignKey(s string) configFunc {
	return func(c *config) error {
		key, err := readKey(s)
		if err != nil {
			return err
		}
		c.sign = key
		return nil
	}
}

// cfDuration sets the duration of cert validity
func cfDuration(d time.Duration) configFunc {
	return func(c *config) error {
		c.until = d
		return nil
	}
}

// cfCA sets that this is a CA cert
func cfCA(t bool) configFunc {
	return func(c *config) error {
		c.ca = t
		return nil
	}
}

// cfKeySize sets the key bit length.
// Returns an error if the value is too small or not divisable by 2.
func cfKeySize(i int) configFunc {
	return func(c *config) error {
		switch {
		case i < keySizeTooSmall:
			return errors.New("key size value is too small (< 512)")
		case i > keySizeTooLarge:
			return errors.New("key size value is too large (> 4096)")
		case i%2 != 0:
			return errors.New("key size value must be a factor of 2")
		}
		c.keySize = i
		return nil
	}
}

// cfOverwrite sets the overwrite flag used to create private keys.
func cfOverwrite(t bool) configFunc {
	return func(c *config) error {
		c.force = t
		return nil
	}
}

// cfHosts sets the hosts (ip addresses or hostnames) that are allowed in a
// node certificate.
// Returns an error if the list is empty.
func cfHosts(h ...string) configFunc {
	return func(c *config) error {
		if len(h) == 0 {
			return errors.New("the hosts list is empty")
		}
		c.hosts = h
		c.ca = false
		return nil
	}
}

// cfUser sets the user for a client certificate, using the Subject CommonName field.
func cfUser(s string) configFunc {
	return func(c *config) error {
		c.user = s
		c.ca = false
		return nil
	}
}

// NewRequest returns a new cert request based on configuration. Each configuration
// value is tested. This function only creates a non-destructive request.
// Returns the new request, or an error otherwise.
func NewRequest(options ...configFunc) (*Request, error) {
	var r Request

	for _, f := range options {
		if err := f(&r.c); err != nil {
			return nil, err
		}
	}

	return &r, nil
}

// GeneratePair makes a new key/cert pair from a request. This function
// will do a best guess of the cert to create based on the request values.
// It will generate two files, a key and cert, upon success.
// Returns nil on success, or an error otherwise.
func (r *Request) GeneratePair(keyFile, crtFile string) error {
	key, err := makeKey(keyFile, r.c.keySize, r.c.force)
	if err != nil {
		return err
	}

	sn, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return err
	}

	template := &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{dnOrganization},
			SerialNumber: hex.EncodeToString(sn.Bytes()[:3]),
		},
		SerialNumber:          sn,
		NotBefore:             time.Now().Add(validNotBefore),
		NotAfter:              time.Now().Add(r.c.until),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
		IsCA:                  r.c.ca,
		MaxPathLenZero:        r.c.ca,
	}

	switch {
	case r.c.ca:
		template.Subject.CommonName = dnOrganization + " Root CA"
		template.KeyUsage |= x509.KeyUsageContentCommitment | x509.KeyUsageCertSign

	case r.c.hosts != nil:
		template.Subject.CommonName = dnOrganization + " Node"
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}

		for _, h := range r.c.hosts {
			if ip := net.ParseIP(h); ip != nil {
				template.IPAddresses = append(template.IPAddresses, ip)
			} else {
				template.DNSNames = append(template.DNSNames, h)
			}
		}

	case r.c.user != "":
		template.Subject.CommonName = r.c.user
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	if r.c.sign == nil {
		r.c.sign = key
	}

	if r.c.parent == nil {
		r.c.parent = template
	}

	der, err := x509.CreateCertificate(rand.Reader,
		template, r.c.parent, key.Public(), r.c.sign)
	if err != nil {
		return err
	}

	f, err := os.Create(crtFile)
	if err != nil {
		return err
	}
	defer f.Close()

	err = pem.Encode(f, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	})
	if err != nil {
		return err
	}

	_, err = x509.ParseCertificate(der)
	return err
}
