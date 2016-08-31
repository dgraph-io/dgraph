/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// TODO(jiawei)
// This is supposed to send out RPC calls if server instance does not handle
// given attribute / predicate.
package index

import (
	"log"
	"os"

	"github.com/dgraph-io/dgraph/x"
)

var (
	globalIndices *Indices
)

// InitWorker initializes Bleve indices for this instance. Returns the Indices
// constructed. You may need this in tests. Most of the time, you can ignore.
func InitWorker(indicesDir string) *Indices {
	// Create indices directory if it is missing.
	err := os.MkdirAll(indicesDir, 0700)
	if err != nil {
		log.Fatalf("Error while creating the filepath for indices: %v", err)
	}

	// Read in the indices.
	globalIndices, err = NewIndices(indicesDir)
	x.Check(err)

	return globalIndices
}

// WorkerLookup looks up the indices and push the result to the given channel.
func WorkerLookup(li *LookupSpec, results chan *LookupResult) {
	// Will check instance here next time.
	results <- globalIndices.Lookup(li)
}
