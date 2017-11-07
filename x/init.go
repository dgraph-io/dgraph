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
	"os"

	yaml "gopkg.in/yaml.v2"
)

var (
	initFunc []func()
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
func Init(debug bool) {
	Config.DebugMode = debug
	// Lets print the details of the current build on startup.
	printBuildDetails()

	// Next, run all the init functions that have been added.
	for _, f := range initFunc {
		f()
	}
}

// loadConfigFromYAML reads configurations from specified YAML file.
func LoadConfigFromYAML(file string) {
	bs, err := ioutil.ReadFile(file)
	Checkf(err, "Cannot open specified config file: %v", file)

	m := make(map[string]string)
	Checkf(yaml.Unmarshal(bs, &m), "Error while parsing config file: %v", Config.ConfigFile)

	for k, v := range m {
		Printf("Picked flag from config: [%q = %v]\n", k, v)
		err := flag.Set(k, v)
		Checkf(err, "While setting flag from config.")
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

// PrintVersionOnly prints version and other helpful information if --version.
func PrintVersionOnly() {
	printBuildDetails()
	fmt.Println()
	fmt.Println("Copyright 2017 Dgraph Labs, Inc.")
	fmt.Println(`
Licensed under AGPLv3.
For Dgraph official documentation, visit https://docs.dgraph.io.
For discussions about Dgraph     , visit https://discuss.dgraph.io.
To say hi to the community       , visit https://dgraph.slack.com.
`)
	os.Exit(0)
}

func Version() string {
	return dgraphVersion
}
