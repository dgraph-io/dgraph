// +build gofuzz

/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package gql

// GQL parser fuzzer for use with https://github.com/dvyukov/go-fuzz.
//
// Build: go-fuzz-build github.com/dgraph-io/dgraph/gql
//
// Run: go-fuzz -bin=./gql-fuzz.zip -workdir fuzz-data

const (
	fuzzInteresting = 1
	fuzzNormal      = 0
	fuzzDiscard     = -1
)

func Fuzz(in []byte) int {
	_, err := Parse(Request{Str: string(in)})
	if err == nil {
		return fuzzInteresting
	}

	return fuzzNormal
}
