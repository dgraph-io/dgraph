package main

import "sort"

func Tokens(s string) ([]string, error) {
	b := []byte(s)
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	return []string{string(b)}, nil
}
