package x

import (
	"flag"
	"os"
	"strings"
	"testing"
)

func someTestFunc() error {
	return Errorf("some nice error")
}

func TestTraceError(t *testing.T) {
	s := shortenedErrorString(someTestFunc())
	if !strings.HasPrefix(s, "some nice error; x.Errorf (x/error.go:90) x.someTestFunc (x/error_test.go:11)") {
		t.Errorf("Error string has wrong prefix: %s", s)
		return
	}
}

func TestMain(m *testing.M) {
	flag.Parse()
	flag.Set("debugmode", "true")
	os.Exit(m.Run())
}
