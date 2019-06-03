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
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type certInfo struct {
	fileName     string
	issuerName   string
	commonName   string
	serialNumber string
	verifiedCA   string
	digest       string
	algo         string
	expireDate   time.Time
	hosts        []string
	fileMode     string
	err          error
}

func getFileInfo(file string) *certInfo {
	var info certInfo
	info.fileName = file

	switch {
	case strings.HasSuffix(file, ".crt"):
		cert, err := readCert(file)
		if err != nil {
			info.err = err
			return &info
		}
		info.commonName = cert.Subject.CommonName + " certificate"
		info.issuerName = strings.Join(cert.Issuer.Organization, ", ")
		info.serialNumber = hex.EncodeToString(cert.SerialNumber.Bytes())
		info.expireDate = cert.NotAfter

		switch {
		case file == defaultCACert:
		case file == defaultNodeCert:
			for _, ip := range cert.IPAddresses {
				info.hosts = append(info.hosts, ip.String())
			}
			for _, name := range cert.DNSNames {
				info.hosts = append(info.hosts, name)
			}

		case strings.HasPrefix(file, "client."):
			info.commonName = fmt.Sprintf("%s client certificate: %s",
				dnCommonNamePrefix, cert.Subject.CommonName)

		default:
			info.err = fmt.Errorf("Unsupported certificate")
			return &info
		}

		switch key := cert.PublicKey.(type) {
		case *rsa.PublicKey:
			info.digest = getHexDigest(key.N.Bytes())
		case *ecdsa.PublicKey:
			info.digest = getHexDigest(elliptic.Marshal(key.Curve, key.X, key.Y))
		default:
			info.digest = "Invalid public key"
		}

		if file != defaultCACert {
			parent, err := readCert(defaultCACert)
			if err != nil {
				info.err = fmt.Errorf("Could not read parent cert: %s", err)
				return &info
			}
			if err := cert.CheckSignatureFrom(parent); err != nil {
				info.verifiedCA = "FAILED"
			}
			info.verifiedCA = "PASSED"
		}

	case strings.HasSuffix(file, ".key"):
		switch {
		case file == defaultCAKey:
			info.commonName = dnCommonNamePrefix + " Root CA key"

		case file == defaultNodeKey:
			info.commonName = dnCommonNamePrefix + " Node key"

		case strings.HasPrefix(file, "client."):
			info.commonName = dnCommonNamePrefix + " Client key"

		default:
			info.err = fmt.Errorf("Unsupported key")
			return &info
		}

		priv, err := readKey(file)
		if err != nil {
			info.err = err
			return &info
		}
		key, ok := priv.(crypto.Signer)
		if !ok {
			info.err = errors.Errorf("Unknown private key type: %T", key)
		}
		switch k := key.(type) {
		case *ecdsa.PrivateKey:
			info.algo = fmt.Sprintf("ECDSA %s (FIPS-3)", k.PublicKey.Curve.Params().Name)
			info.digest = getHexDigest(elliptic.Marshal(k.PublicKey.Curve,
				k.PublicKey.X, k.PublicKey.Y))
		case *rsa.PrivateKey:
			info.algo = fmt.Sprintf("RSA %d bits (PKCS#1)", k.PublicKey.N.BitLen())
			info.digest = getHexDigest(k.PublicKey.N.Bytes())
		}

	default:
		info.err = fmt.Errorf("Unsupported file")
	}

	return &info
}

// getHexDigest returns a SHA-256 hex digest broken up into 32-bit chunks
// so that they easier to compare visually
// e.g. 4A2B0F0F 716BF5B6 C603E01A 6229D681 0B2AFDC5 CADF5A0D 17D59299 116119E5
func getHexDigest(data []byte) string {
	const groupSizeBytes = 4

	digest := sha256.Sum256(data)
	groups := len(digest) / groupSizeBytes
	hex := fmt.Sprintf("%0*X", groupSizeBytes*2, digest[0:groupSizeBytes])
	for i := 1; i < groups; i++ {
		hex += fmt.Sprintf(" %0*X", groupSizeBytes*2,
			digest[i*groupSizeBytes:(i+1)*groupSizeBytes])
	}

	return hex
}

// getDirFiles walks dir and collects information about the files contained.
// Returns the list of files, or an error otherwise.
func getDirFiles(dir string) ([]*certInfo, error) {
	if err := os.Chdir(dir); err != nil {
		return nil, err
	}

	var files []*certInfo
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ci := getFileInfo(path)
		if ci == nil {
			return nil
		}
		ci.fileMode = info.Mode().String()
		files = append(files, ci)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

// Format implements the fmt.Formatter interface, used by fmt functions to
// generate output using custom format specifiers. This function creates the
// format specifiers '%n', '%x', '%e' to extract name, expiration date, and
// error string from an Info object.
func (i *certInfo) Format(f fmt.State, c rune) {
	w, wok := f.Width()     // width modifier. eg., %20n
	p, pok := f.Precision() // precision modifier. eg., %.20n

	var str string
	switch c {
	case 'n':
		str = i.commonName

	case 'x':
		if i.expireDate.IsZero() {
			break
		}
		str = i.expireDate.Format(time.RFC822)

	case 'e':
		if i.err != nil {
			str = i.err.Error()
		}
	}

	if wok {
		str = fmt.Sprintf("%-[2]*[1]s", str, w)
	}
	if pok && len(str) < p {
		str = str[:p]
	}

	f.Write([]byte(str))
}
