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
