// +build gofuzz

/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
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
