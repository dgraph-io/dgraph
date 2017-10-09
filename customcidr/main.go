package main

import (
	"net"
)

var Name string = "cidr"

func Tokens(s string) ([]string, error) {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}
	return []string{network.String()}, nil
}
