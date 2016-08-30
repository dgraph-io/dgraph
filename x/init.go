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
// package x provides some common utilities used by the entire library.
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
	if !flag.Parsed() {
		log.Fatal("Unable to parse flags")
	}
	printVersionOnly()
	// Next, run all the init functions that have been added.
	for _, f := range initFunc {
		f()
	}
}

// printVersionOnly prints version and other helpful information if --version.
func printVersionOnly() {
	if *version {
		fmt.Printf("Dgraph version %s\n", dgraphVersion)
		fmt.Println("\nCopyright 2016 Dgraph Labs, Inc.")
		fmt.Println("Licensed under the Apache License, Version 2.0.")
		fmt.Println("\nFor Dgraph official documentation, visit https://wiki.dgraph.io.")
		fmt.Println("For discussions about Dgraph, visit https://discuss.dgraph.io.")
		fmt.Println("To say hi to the community, visit https://dgraph.slack.com.")
		os.Exit(0)
	}
}
