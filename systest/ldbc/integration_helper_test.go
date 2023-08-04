//go:build integration

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"github.com/dgraph-io/dgraph/testutil"
)

func (lsuite *LdbcTestSuite) bulkLoader() error {
	noschemaFile := filepath.Join(testutil.TestDataDirectory, "ldbcTypes.schema")
	rdfFile := testutil.TestDataDirectory
	require.NoError(t, testutil.MakeDirEmpty([]string{"out/0"}))
	start := time.Now()
	return testutil.BulkLoad(testutil.BulkOpts{
		Zero:       testutil.SockAddrZero,
		Shards:     1,
		RdfFile:    rdfFile,
		SchemaFile: noschemaFile,
	})
	fmt.Printf("Took %s to bulkupload LDBC dataset\n", time.Since(start))
}

func (lsuite *LdbcTestSuite) StartAlphas() error {
	return testutil.StartAlphas("./alpha.yml")
}

func (lsuite *LdbcTestSuite) StopAlphasForCoverage() {
	testutil.StopAlphasForCoverage("./alpha.yml")
}
