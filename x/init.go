package x

import (
	"flag"
	"fmt"
	"log"
	"os"
)

const dgraphVersion = "0.4.3"

var (
	version  = flag.Bool("version", false, "Prints the version of Dgraph")
	initFunc []func()
)

// AddInit adds a function to be run in x.Init, which should be called at the
// beginning of all mains.
func AddInit(f func()) {
	initFunc = append(initFunc, f)
}

// Init initializes flags and run all functions in initFunc.
func Init() {
	log.SetFlags(log.Lshortfile | log.Flags())
	flag.Parse()
	Assertf(flag.Parsed(), "Unable to parse flags")
	if printVersionOnly() {
		os.Exit(0)
	}
	for _, f := range initFunc {
		f()
	}
}

// PrintVersionOnly prints version and other helpful information if version flag
// is set to true.
func printVersionOnly() bool {
	if *version {
		fmt.Printf("Dgraph version %s\n", dgraphVersion)
		fmt.Println("\nCopyright 2016 Dgraph Labs, Inc.")
		fmt.Println("Licensed under the Apache License, Version 2.0.")
		fmt.Println("\nFor Dgraph official documentation, visit https://wiki.dgraph.io.")
		fmt.Println("For discussions about Dgraph, visit https://discuss.dgraph.io.")
		fmt.Println("To say hi to the community, visit https://dgraph.slack.com.")
		return true
	}
	return false
}
