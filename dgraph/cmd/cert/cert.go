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
	dnOrganization     = "Dgraph Labs, Inc."
	dnCommonNamePrefix = "Dgraph"
)

type certConfig struct {
	parent  *x509.Certificate
	signer  crypto.Signer
	until   int
	isCA    bool
	keySize int
	force   bool
	hosts   []string
	client  string
	curve   string
}

// generatePair makes a new key/cert pair from a request. This function
// will do a best guess of the cert to create based on the certConfig values.
// It will generate two files, a key and cert, upon success.
// Returns nil on success, or an error otherwise.
func (c *certConfig) generatePair(keyFile, certFile string) error {
	priv, err := makeKey(keyFile, c)
	if err != nil {
		return err
	}
	key := priv.(crypto.Signer)

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
		NotBefore:             time.Now().AddDate(0, 0, -1).UTC(),
		NotAfter:              time.Now().AddDate(0, 0, c.until).UTC(),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: c.isCA,
		IsCA:                  c.isCA,
		MaxPathLenZero:        c.isCA,
	}

	switch {
	case c.isCA:
		template.Subject.CommonName = dnCommonNamePrefix + " Root CA"
		template.KeyUsage |= x509.KeyUsageContentCommitment | x509.KeyUsageCertSign

	case c.hosts != nil:
		template.Subject.CommonName = dnCommonNamePrefix + " Node"
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth}

		for _, h := range c.hosts {
			if ip := net.ParseIP(h); ip != nil {
				template.IPAddresses = append(template.IPAddresses, ip)
			} else {
				template.DNSNames = append(template.DNSNames, h)
			}
		}

	case c.client != "":
		template.Subject.CommonName = c.client
		template.KeyUsage = x509.KeyUsageDigitalSignature
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

	fp, err := safeCreate(certFile, c.force, 0666)
	if err != nil {
		// check the existing cert.
		if os.IsExist(err) {
			_, err = readCert(certFile)
		}
		return err
	}
	defer fp.Close()

	err = pem.Encode(fp, &pem.Block{
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
		opts.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth}
		for i := range c.hosts {
			if err := cert.VerifyHostname(c.hosts[i]); err != nil {
				return err
			}
		}
	}
	if c.client != "" {
		opts.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	_, err = cert.Verify(opts)
	if err != nil {
		return fmt.Errorf("%s: verification failed: %s", certFile, err)
	}

	return nil
}
