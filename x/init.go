/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/golang/glog"
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

// SetTestRun sets a variable to indicate that the current execution is a test.
func SetTestRun() {
	isTest = true
}

// IsTestRun indicates whether a test is being executed. Useful to handle special
// conditions during tests that differ from normal execution.
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
	// Default value, would be overwritten by flag.
	Config.QueryEdgeLimit = 1e6

	// Next, run all the init functions that have been added.
	for _, f := range initFunc {
		f()
	}
}

// BuildDetails returns a string containing details about the Dgraph binary.
func BuildDetails() string {
	licenseInfo := `Licensed under the Apache Public License 2.0`
	if !strings.HasSuffix(dgraphVersion, "-oss") {
		licenseInfo = `Licensed variously under the Apache Public License 2.0 and Dgraph Community License`
	}
	return fmt.Sprintf(`
Dgraph version   : %v
Commit SHA-1     : %v
Commit timestamp : %v
Branch           : %v
Go version       : %v

For Dgraph official documentation, visit https://docs.dgraph.io.
For discussions about Dgraph     , visit https://discuss.dgraph.io.
To say hi to the community       , visit https://dgraph.slack.com.

%s.
Copyright 2015-2018 Dgraph Labs, Inc.

`,
		dgraphVersion, lastCommitSHA, lastCommitTime, gitBranch, runtime.Version(), licenseInfo)
}

// PrintVersion prints version and other helpful information if --version.
func PrintVersion() {
	glog.Infof("\n%s\n", BuildDetails())
}

// Version returns a string containing the dgraphVersion.
func Version() string {
	return dgraphVersion
}
