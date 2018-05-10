/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package x

import (
	"fmt"
	"log"
	"os"
)

var (
	Logger = log.New(os.Stderr, "", log.Lshortfile|log.Flags())
)

// Printf does a log.Printf. We often do printf for debugging but has to keep
// adding import "fmt" or "log" and removing them after we are done.
// Let's add Printf to "x" and include "x" almost everywhere. Caution: Do remember
// to call x.Init. For tests, you need a TestMain that calls x.Init.
func Printf(format string, args ...interface{}) {
	Logger.Output(2, fmt.Sprintf(format, args...))
}

func Println(args ...interface{}) {
	Logger.Output(2, fmt.Sprintln(args...))
}
