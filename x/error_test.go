/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package x

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

const originalError = `some nice error
github.com/dgraph-io/dgraph/x.Errorf
	/home/jchiu/go/src/github.com/dgraph-io/dgraph/x/error.go:90
github.com/dgraph-io/dgraph/x.someTestFunc
	/home/jchiu/go/src/github.com/dgraph-io/dgraph/x/error_test.go:12
github.com/dgraph-io/dgraph/x.TestTraceError
	/home/jchiu/go/src/github.com/dgraph-io/dgraph/x/error_test.go:16
testing.tRunner
	/usr/lib/go-1.7/src/testing/testing.go:610
runtime.goexit
	/usr/lib/go-1.7/src/runtime/asm_amd64.s:2086`

const expectedError = `some nice error; x.Errorf (x/error.go:90) x.someTestFunc (x/error_test.go:12) x.TestTraceError (x/error_test.go:16) testing.tRunner (testing.go:610) runtime.goexit (asm_amd64.s:2086) `

func TestTraceError(t *testing.T) {
	s := shortenedErrorString(fmt.Errorf(originalError))
	if s != expectedError {
		t.Errorf("Error string is wrong: [%s]", s)
		return
	}
}

func TestMain(m *testing.M) {
	flag.Parse()
	flag.Set("debugmode", "true")
	os.Exit(m.Run())
}
