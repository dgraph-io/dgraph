/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package x

import (
	"flag"
	"fmt"
	"log"
	"os"

	yaml "gopkg.in/yaml.v2"
)

const dgraphVersion = "0.7.0"

var (
	version  = flag.Bool("version", false, "Prints the version of Dgraph")
	initFunc []func()
	logger   *log.Logger
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

	var confFile = flag.Lookup("conf").Value.(flag.Getter).Get().(string)

	if confFile == "" {
		log.Println("No config file specified! Default configuration will be used.")
	} else {
		log.Println("Trying to load configuration from file:", confFile)
		loadConfigFromYAML(confFile)
	}

	logger = log.New(os.Stderr, "", log.Lshortfile|log.Flags())
	AssertTrue(logger != nil)
	printVersionOnly()
	// Next, run all the init functions that have been added.
	for _, f := range initFunc {
		f()
	}
}

// loadConfigFromYAML reads configurations from specified YAML file.
func loadConfigFromYAML(confFile string) {
	file, err := os.Open(confFile)
	if err != nil {
		// handle the error here
		log.Println("Cannot open specified config file. Default configuration will be used")
		return
	}

	defer file.Close()

	// get the file size
	stat, err := file.Stat()
	if err != nil {
		log.Println("Cannot read config file.")
		return
	}

	// read the file
	bs := make([]byte, stat.Size())

	_, err = file.Read(bs)
	if err != nil {
		log.Println("Error parsing config file.")
		return
	}

	m := make(map[string]string)

	err = yaml.Unmarshal([]byte(bs), &m)
	if err != nil {
		log.Println("Error in marshaling config file.")
		return
	}

	for k, v := range m {
		flag.Set(k, v)
	}
}

// printVersionOnly prints version and other helpful information if --version.
func printVersionOnly() {
	if *version {
		fmt.Printf("Dgraph version %s\n", dgraphVersion)
		fmt.Println("\nCopyright 2016 Dgraph Labs, Inc.")
		fmt.Println(`
Licensed under the Apache License, version 2.0.
For Dgraph official documentation, visit https://wiki.dgraph.io.
For discussions about Dgraph     , visit https://discuss.dgraph.io.
To say hi to the community       , visit https://dgraph.slack.com.
`)
		os.Exit(0)
	}
}

// Printf does a log.Printf. We often do printf for debugging but has to keep
// adding import "fmt" or "log" and removing them after we are done.
// Let's add Printf to "x" and include "x" almost everywhere. Caution: Do remember
// to call x.Init. For tests, you need a TestMain that calls x.Init.
func Printf(format string, args ...interface{}) {
	AssertTruef(logger != nil, "Logger is not defined. Have you called x.Init?")
	// Call depth is one higher than default.
	logger.Output(2, fmt.Sprintf(format, args...))
}
