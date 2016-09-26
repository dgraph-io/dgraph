package main

import (
	"context"

	"github.com/dgraph-io/dgraph/x"
)

func f() error {
	return x.Wrapf(x.Errorf("some error"), "somewrap")
}

func main() {
	x.Init()
	e := f()
	x.TraceError(context.Background(), e)
}
