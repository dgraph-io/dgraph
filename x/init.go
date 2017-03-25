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
	"io/ioutil"
	"log"
	"os"

	yaml "gopkg.in/yaml.v2"
)

var (
	configFile = flag.String("config", "",
		"YAML configuration file containing dgraph settings.")
	version  = flag.Bool("version", false, "Prints the version of Dgraph")
	initFunc []func()
	logger   *log.Logger
	isTest   bool

	// These variables are set using -ldflags
	dgraphVersion  string
	gitBranch      string
	lastCommitSHA  string
	lastCommitTime string
)

func SetTestRun() {
	isTest = true
}

func IsTestRun() bool {
	return isTest
}

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

	// Lets print the details of the current build on startup.
	printBuildDetails()

	if *configFile != "" {
		log.Println("Loading configuration from file:", *configFile)
		loadConfigFromYAML()
	}

	logger = log.New(os.Stderr, "", log.Lshortfile|log.Flags())
	AssertTrue(logger != nil)

	// Next, run all the init functions that have been added.
	for _, f := range initFunc {
		f()
	}
}

// loadConfigFromYAML reads configurations from specified YAML file.
func loadConfigFromYAML() {
	bs, err := ioutil.ReadFile(*configFile)
	Checkf(err, "Cannot open specified config file: %v", *configFile)

	m := make(map[string]string)
	Checkf(yaml.Unmarshal(bs, &m), "Error while parsing config file: %v", *configFile)

	for k, v := range m {
		fmt.Printf("Picked flag from config: [%q = %v]\n", k, v)
		flag.Set(k, v)
	}
}

func printBuildDetails() {
	if dgraphVersion == "" {
		return
	}

	fmt.Printf(fmt.Sprintf(`
Dgraph version   : %v
Commit SHA-1     : %v
Commit timestamp : %v
Branch           : %v`,
		dgraphVersion, lastCommitSHA, lastCommitTime, gitBranch) + "\n\n")
}

// printVersionOnly prints version and other helpful information if --version.
func printVersionOnly() {
	if *version {
		printBuildDetails()
		fmt.Println("Copyright 2017 Dgraph Labs, Inc.")
		fmt.Println(`
Licensed under AGPLv3.
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
