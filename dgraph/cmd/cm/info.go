/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cm

import (
	"fmt"
	"strings"
	"time"
)

type Info struct {
	Name    string
	Expires time.Time
	Err     error
}

func certInfo(fn string) *Info {
	var i Info

	switch {
	case strings.HasSuffix(fn, ".crt"):
		cert, _ := readCert(fn)
		i.Name = cert.Subject.CommonName
		i.Expires = cert.NotAfter

	case strings.HasSuffix(fn, ".key"):
		switch {
		case strings.HasPrefix(fn, "ca."):
			i.Name = dnOrganization + " CA Key"
		case strings.HasPrefix(fn, "node."):
			i.Name = dnOrganization + " Node Key"
		case strings.HasPrefix(fn, "client."):
			i.Name = dnOrganization + " Client Key"
		}
	default:
		return nil
	}

	return &i
}

func (i *Info) Format(f fmt.State, c rune) {
	var str string

	w, wok := f.Width()
	p, pok := f.Precision()

	switch c {
	case 't':
		str = i.Name

	case 'x':
		if i.Expires.IsZero() {
			break
		}
		str = i.Expires.Format(time.RFC822)

	case 'e':
		if i.Err != nil {
			str = i.Err.Error()
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
