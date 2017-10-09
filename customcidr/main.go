package main

import (
	"fmt"
	"net"
)

var (
	Name       string = "cidr"
	Identifier byte   = 0x42
	Sortable   bool   = false
	IsLossy    bool   = true
)

func Tokens(s string) ([]string, error) {
	fmt.Printf("TOKENIZING: %q\n", s)
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		fmt.Printf("COULD NOT TOKENIZE: %v\n", err)
		return nil, err
	}
	fmt.Printf("RESULT: %q\n", network.String())
	return []string{network.String()}, nil
}
