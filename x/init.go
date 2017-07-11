/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
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
	"io/ioutil"
	"log"
	"os"

	yaml "gopkg.in/yaml.v2"
)

var (
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

	printVersionOnly()

	// Lets print the details of the current build on startup.
	printBuildDetails()

	if Config.ConfigFile != "" {
		log.Println("Loading configuration from file:", Config.ConfigFile)
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
	bs, err := ioutil.ReadFile(Config.ConfigFile)
	Checkf(err, "Cannot open specified config file: %v", Config.ConfigFile)

	m := make(map[string]string)
	Checkf(yaml.Unmarshal(bs, &m), "Error while parsing config file: %v", Config.ConfigFile)

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
	if Config.Version {
		printBuildDetails()
		fmt.Println("Copyright 2017 Dgraph Labs, Inc.")
		fmt.Println(`
Licensed under AGPLv3.
For Dgraph official documentation, visit https://docs.dgraph.io.
For discussions about Dgraph     , visit https://discuss.dgraph.io.
To say hi to the community       , visit https://dgraph.slack.com.
`)
		os.Exit(0)
	}
}

func Version() string {
	return dgraphVersion
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
