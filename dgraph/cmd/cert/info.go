/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cert

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type certInfo struct {
	fileName     string
	issuer       string
	commonName   string
	serialNumber string
	verified     bool
	md5sum       string
	keyFile      string
	expires      time.Time
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
		info.commonName = cert.Subject.CommonName
		info.issuer = cert.Issuer.CommonName
		info.serialNumber = hex.EncodeToString(cert.SerialNumber.Bytes())
		info.expires = cert.NotAfter

		switch {
		case file == defaultCACert:
			info.keyFile = defaultCAKey

		case file == defaultNodeCert:
			info.keyFile = defaultNodeKey
			for _, ip := range cert.IPAddresses {
				info.hosts = append(info.hosts, ip.String())
			}
			for _, name := range cert.DNSNames {
				info.hosts = append(info.hosts, name)
			}

		default:
			info.err = fmt.Errorf("unsupported certificate")
		}

	case strings.HasSuffix(file, ".key"):
		switch {
		case file == defaultCAKey:
			info.commonName = dnOrganization + " Root CA key"
		case file == defaultNodeKey:
			info.commonName = dnOrganization + " Node key"
		default:
			info.err = fmt.Errorf("unsupported key")
		}

	default:
		info.err = fmt.Errorf("unsupported file")
	}

	return &info
}

// getDirFiles walks dir and collects information about the files contained.
// Returns the list of files, or an error otherwise.
func getDirFiles(dir string) ([]*certInfo, error) {
	var files []*certInfo

	if err := os.Chdir(dir); err != nil {
		return nil, err
	}

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
		if i.expires.IsZero() {
			break
		}
		str = i.expires.Format(time.RFC822)

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
