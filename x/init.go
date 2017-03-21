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
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

var (
	configFile = flag.String("config", "",
		"YAML configuration file containing dgraph settings.")
	version  = flag.Bool("version", false, "Prints the version of Dgraph")
	initFunc []func()
	logger   *log.Logger
	isTest   bool
	bd       buildDetails
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

	extractBuildDetails()
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
	fmt.Printf(fmt.Sprintf(`Dgraph version   : %v
Commit SHA-1     : %v
Commit timestamp : %v
Branch       	 : %v`,
		bd.dgraphVersion, bd.lastCommitSHA, bd.lastCommitTime, bd.gitBranch) + "\n\n")
}

// printVersionOnly prints version and other helpful information if --version.
func printVersionOnly() {
	if *version {
		printBuildDetails()
		fmt.Println("Copyright 2016 Dgraph Labs, Inc.")
		fmt.Println(`
Licensed under the Apache License, version 2.0.
For Dgraph official documentation, visit https://wiki.dgraph.io.
For discussions about Dgraph     , visit https://discuss.dgraph.io.
To say hi to the community       , visit https://dgraph.slack.com.
`)
		os.Exit(0)
	}
}

type buildDetails struct {
	dgraphVersion  string
	gitBranch      string
	lastCommitSHA  string
	lastCommitTime string
}

func runCommand(args []string) string {
	output, err := exec.Command(args[0], args[1:]...).Output()
	Check(err)
	return strings.TrimSpace(string(output))
}

func extractBuildDetails() {
	// `git describe --abbrev=0` returns the latest release tag.
	bd.dgraphVersion = runCommand([]string{"git", "describe", "--abbrev=0"})
	// `git rev-parse --short HEAD` is used to get the short form of the SHA1 of
	// the latest commit.
	bd.lastCommitSHA = runCommand([]string{"git", "rev-parse", "--short", "HEAD"})
	bd.gitBranch = runCommand([]string{"git", "rev-parse", "--abbrev-ref", "HEAD"})
	// `git log -1` is the latest comitt. $ci is a format option. Other options
	// can be found here. https://git-scm.com/docs/pretty-formats
	bd.lastCommitTime = runCommand([]string{"git", "log", "-1", `--format=%ci`})
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
