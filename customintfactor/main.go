package main

import (
	"fmt"
	"strconv"
)

var Name string = "intfactor"

func Tokens(s string) ([]string, error) {
	x, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	if x <= 1 {
		return nil, fmt.Errorf("cannot factor negative integer: %d", x)
	}
	var toks []string
	for p := 2; x > 1; p++ {
		if x%p == 0 {
			toks = append(toks, strconv.Itoa(p))
			for x%p == 0 {
				x /= p
			}
		}
	}
	return toks, nil
}
