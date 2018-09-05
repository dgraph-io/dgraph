/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cert

import (
	"fmt"
	"strings"
	"time"
)

type Info struct {
	Type string
	File string
	Exp  time.Time
	Err  error
}

func certInfo(fn string) *Info {
	var i Info

	i.File = fn

	switch {
	case strings.HasSuffix(fn, ".crt"):
		cert, _ := readCert(fn)
		i.Exp = cert.NotAfter

		switch {
		case cert.IsCA && strings.HasPrefix(fn, "ca."):
			i.Type = "CA Cert"
		case cert.IsCA && strings.HasPrefix(fn, "ca-client"):
			i.Type = "Client CA Cert"
		case strings.HasPrefix(fn, "node"):
			i.Type = "Node Cert"
		case strings.HasPrefix(fn, "client-"):
			i.Type = "Client CA Cert"
		}

	case strings.HasSuffix(fn, ".key"):
		switch {
		case strings.HasPrefix(fn, "ca."):
			i.Type = "CA Key"
		case strings.HasPrefix(fn, "ca-client"):
			i.Type = "Client CA Key"
		case strings.HasPrefix(fn, "node"):
			i.Type = "Node Key"
		case strings.HasPrefix(fn, "client-"):
			i.Type = "Client CA Key"
		}
	}

	return &i
}

func (i *Info) Format(f fmt.State, c rune) {
	var str string

	w, wok := f.Width()
	p, pok := f.Precision()

	switch c {
	case 't':
		str = i.Type

	case 'f':
		str = i.File

	case 'x':
		if i.Exp.IsZero() {
			break
		}
		str = i.Exp.Format(time.RFC822)

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
