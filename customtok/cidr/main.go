package main

import (
	"net"
)

func Tokens(s string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}
	ones, bits := ipnet.Mask.Size()
	var toks []string
	for i := ones; i >= 1; i-- {
		m := net.CIDRMask(i, bits)
		tok := net.IPNet{
			IP:   ipnet.IP.Mask(m),
			Mask: m,
		}
		toks = append(toks, tok.String())
	}
	return toks, nil
}
