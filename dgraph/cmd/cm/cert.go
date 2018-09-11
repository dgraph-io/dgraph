/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cm

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math"
	"math/big"
	"net"
	"os"
	"time"
)

const (
	dnOrganization = "Dgraph"
	validNotBefore = time.Hour * -24
)

type certConfig struct {
	parent  *x509.Certificate
	signer  crypto.Signer
	until   int
	isCA    bool
	keySize int
	force   bool
	hosts   []string
	user    string
}

// generatePair makes a new key/cert pair from a request. This function
// will do a best guess of the cert to create based on the certConfig values.
// It will generate two files, a key and cert, upon success.
// Returns nil on success, or an error otherwise.
func (c *certConfig) generatePair(keyFile, certFile string) error {
	key, err := makeKey(keyFile, c.keySize, c.force)
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
		NotBefore:             time.Now().AddDate(0, 0, -1),
		NotAfter:              time.Now().AddDate(0, 0, c.until),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
		IsCA:                  c.isCA,
		MaxPathLenZero:        c.isCA,
	}

	switch {
	case c.isCA:
		template.Subject.CommonName = dnOrganization + " Root CA"
		template.KeyUsage |= x509.KeyUsageContentCommitment | x509.KeyUsageCertSign

	case c.hosts != nil:
		template.Subject.CommonName = dnOrganization + " Node"
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}

		for _, h := range c.hosts {
			if ip := net.ParseIP(h); ip != nil {
				template.IPAddresses = append(template.IPAddresses, ip)
			} else {
				template.DNSNames = append(template.DNSNames, h)
			}
		}

	case c.user != "":
		template.Subject.CommonName = c.user
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	if c.signer == nil {
		c.signer = key
	}

	if c.parent == nil {
		c.parent = template
	} else if template.NotAfter.After(c.parent.NotAfter) {
		return fmt.Errorf("--duration: certificate expiration date '%s' exceeds parent '%s'",
			template.NotAfter, c.parent.NotAfter)
	}

	der, err := x509.CreateCertificate(rand.Reader,
		template, c.parent, key.Public(), c.signer)
	if err != nil {
		return err
	}

	f, err := safeCreate(certFile, c.force, 0666)
	if err != nil {
		// check the existing cert.
		if os.IsExist(err) {
			_, err = readCert(certFile)
		}
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

// verifyCert loads a X509 certificate and verifies it against a parent cert (CA).
// If the cert is mapping hosts, it will verify each host individually.
// Returns nil on success, an error otherwise.
func (c *certConfig) verifyCert(certFile string) error {
	cert, err := readCert(certFile)
	if err != nil {
		return err
	}

	if c.isCA || c.parent == nil {
		return nil
	}

	roots := x509.NewCertPool()
	roots.AddCert(c.parent)

	opts := x509.VerifyOptions{Roots: roots}

	if c.hosts != nil {
		for i := range c.hosts {
			if err := cert.VerifyHostname(c.hosts[i]); err != nil {
				return err
			}
		}
	}

	_, err = cert.Verify(opts)
	if err != nil {
		return fmt.Errorf("%s: verification failed: %s", certFile, err)
	}

	return nil
}

// safeCreate only creates a file if it doesn't exist or we force overwrite.
func safeCreate(fn string, overwrite bool, perm os.FileMode) (*os.File, error) {
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flag |= os.O_EXCL
	}
	return os.OpenFile(fn, flag, perm)
}
