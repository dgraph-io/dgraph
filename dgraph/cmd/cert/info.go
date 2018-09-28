/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cert

import (
	"crypto/md5"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type certInfo struct {
	fileName     string
	issuerName   string
	commonName   string
	serialNumber string
	verifiedCA   string
	md5sum       string
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
			info.err = fmt.Errorf("unsupported certificate")
			return &info
		}

		if key, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			h := md5.Sum(key.N.Bytes())
			info.md5sum = fmt.Sprintf("%X", h[:])
		} else {
			info.md5sum = "Invalid RSA public key"
		}

		if file != defaultCACert {
			parent, err := readCert(defaultCACert)
			if err != nil {
				info.err = fmt.Errorf("could not read parent cert: %s", err)
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
			info.err = fmt.Errorf("unsupported key")
			return &info
		}

		key, err := readKey(file)
		if err != nil {
			info.err = err
			return &info
		}
		h := md5.Sum(key.PublicKey.N.Bytes())
		info.md5sum = fmt.Sprintf("%X", h[:])

	default:
		info.err = fmt.Errorf("unsupported file")
	}

	return &info
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
