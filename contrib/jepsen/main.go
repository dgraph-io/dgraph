/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

// Runs Dgraph Jepsen tests with a local Dgraph binary.
// Set JEPSEN_ROOT environment variable before running.
//
// Example usage:
//
// # Runs all test and nemesis combinations (36 total)
// ./jepsen

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type JepsenTest struct {
	workload          string
	nemesis           string
	timeLimit         int
	concurrency       string
	rebalanceInterval string
	localBinary       string
	nodes             string
	skew              string
	testCount         int
}

const (
	TestPass = iota
	TestFail
	TestIncomplete
)

var (
	defaultWorkloads = []string{
		"bank",
		"delete",
		"long-fork",
		"linearizable-register",
		"uid-linearizable-register",
		"upsert",
		"set",
		"uid-set",
		"sequential",
	}
	defaultNemeses = []string{
		"none",
		"kill-alpha,kill-zero",
		"partition-ring",
		"move-tablet",
	}
)

var (
	ctxb = context.Background()

	// Jepsen test flags
	timeLimit         = flag.Int("jepsen.time-limit", 600, "Time limit per Jepsen test in seconds.")
	nodes             = flag.String("jepsen.nodes", "n1,n2,n3,n4,n5", "Nodes to run on.")
	concurrency       = flag.String("jepsen.concurrency", "6n", "Number of concurrent workers.")
	workload          = flag.String("jepsen.workload", strings.Join(defaultWorkloads, ","), "Test workload to run.")
	nemesis           = flag.String("jepsen.nemesis", strings.Join(defaultNemeses, " "), "A space-separated, comma-separated list of nemesis types.")
	localBinary       = flag.String("jepsen.local-binary", "/gobin/dgraph", "Path to local Dgraph binary.")
	rebalanceInterval = flag.String("jepsen.rebalance-interval", "10h", "Interval of Dgraph's tablet rebalancing.")
	skew              = flag.String("jepsen.skew", "", "Skew clock (tiny, small, big, huge)")
	jaeger            = flag.String("jepsen.dgraph-jaeger-collector", "http://jaeger:14268", "Run with Jaeger collector. Set to empty string to disable.")
	testCount         = flag.Int("jepsen.test-count", 1, "Test count per Jepsen test.")

	// Jepsen control flags
	doUp    = flag.Bool("up", true, "Run Jepsen ./up.sh.")
	doServe = flag.Bool("serve", true, "Serve the test results page (lein run serve).")

	// Debug flags
	dryRun = flag.Bool("dry-run", false, "Echo commands that would run, but don't execute them.")
)

func Command(command ...string) *exec.Cmd {
	if *dryRun {
		return exec.Command("echo", command...)
	}
	return exec.Command(command[0], command[1:]...)
}

func CommandContext(ctx context.Context, command ...string) *exec.Cmd {
	if *dryRun {
		return exec.CommandContext(ctx, "echo", command...)
	}
	return exec.CommandContext(ctx, command[0], command[1:]...)
}

func jepsenUp() {
	cmd := Command("./up.sh", "--dev", "--daemon", "--compose", "../dgraph/docker/docker-compose.yml")
	cmd.Dir = os.Getenv("JEPSEN_ROOT") + "/docker/"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func jepsenServe() {
	cmd := Command(
		"docker", "exec", "--workdir", "/jepsen/dgraph", "jepsen-control",
		"lein", "run", "serve")
	// Ignore output and errors. It's okay if "lein run serve" already ran before.
	cmd.Run()
}

func runJepsenTest(test *JepsenTest) int {
	command := []string{
		// setup commands
		"source", "~/.bashrc", "&&",
		"cd", "/jepsen/dgraph", "&&",
		// test commands
		"lein", "run", "test",
		"--workload", test.workload,
		"--nemesis", test.nemesis,
		"--time-limit", strconv.Itoa(test.timeLimit),
		"--concurrency", test.concurrency,
		"--rebalance-interval", test.rebalanceInterval,
		"--local-binary", test.localBinary,
		"--nodes", test.nodes,
		"--test-count", strconv.Itoa(test.testCount),
	}
	if test.nemesis == "skew-clock" {
		command = append(command, "--skew", test.skew)
	}
	if *jaeger != "" {
		command = append(command, "--dgraph-jaeger-collector", *jaeger)
	}

	// Timeout should be a bit longer than the Jepsen test time limit to account
	// for post-analysis time.
	commandTimeout := 10*time.Minute + time.Duration(test.timeLimit)*time.Second
	ctx, _ := context.WithTimeout(ctxb, commandTimeout)
	cmd := CommandContext(ctx, "docker", "exec", "jepsen-control",
		"/bin/bash", "-c", strings.Join(command, " "))

	var out bytes.Buffer
	var stdout io.Writer
	var stderr io.Writer
	stdout = io.MultiWriter(&out, os.Stdout)
	stderr = io.MultiWriter(&out, os.Stderr)
	if inCi() {
		// Jepsen test output is not needed in TeamCity.
		stdout = &out
		stderr = &out
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		// TODO The exit code could probably be checked instead of checking the output.
		// Check jepsen source to be sure.
		// 0 = pass, 1 = failed, 255 = incomplete
		// Pass status: https://github.com/jepsen-io/jepsen/blob/master/jepsen/src/jepsen/cli.clj#L298-L300
		// Fail status: https://github.com/jepsen-io/jepsen/blob/master/jepsen/src/jepsen/cli.clj#L363-L364
		// Incomplete status: https://github.com/jepsen-io/jepsen/blob/master/jepsen/src/jepsen/cli.clj#L302-L304
		if strings.Contains(out.String(), "Analysis invalid") {
			return TestFail
		} else {
			return TestIncomplete
		}
	}
	if strings.Contains(out.String(), "Everything looks good!") {
		return TestPass
	}
	return TestIncomplete
}

func inCi() bool {
	return os.Getenv("TEAMCITY_VERSION") != ""
}

func tcStart(testName string) func(pass int) {
	if !inCi() {
		return nil
	}
	now := time.Now()
	fmt.Printf("##teamcity[testStarted name='%v']\n", testName)
	return func(pass int) {
		durMs := time.Since(now).Nanoseconds() / 1e6
		switch pass {
		case TestPass:
			fmt.Printf("##teamcity[testFinished name='%v' duration='%v']\n", testName, durMs)
		case TestFail:
			fmt.Printf("##teamcity[testFailed='%v' duration='%v']\n", testName, durMs)
		case TestIncomplete:
			fmt.Printf("##teamcity[testFailed='%v' duration='%v' message='Test incomplete.']\n", testName, durMs)
		}
	}
}

func main() {
	flag.Parse()

	if os.Getenv("JEPSEN_ROOT") == "" {
		log.Fatal("JEPSEN_ROOT must be set.")
	}
	if os.Getenv("GOPATH") == "" {
		log.Fatal("GOPATH must be set.")
	}

	if strings.Contains(*nemesis, "skew-clock") && *skew == "" {
		log.Fatal("skew-clock nemesis specified but --jepsen.skew wasn't set.")
	}

	workloads := strings.Split(*workload, ",")
	nemeses := strings.Split(*nemesis, " ")

	numTests := len(workloads) * len(nemeses)
	fmt.Printf("Num tests: %v\n", numTests)

	if *doUp {
		jepsenUp()
	}
	if *doServe {
		go jepsenServe()
	}

	for _, n := range nemeses {
		for _, w := range workloads {
			tcEnd := tcStart(fmt.Sprintf("Workload:%v,Nemeses:%v", w, n))
			pass := runJepsenTest(&JepsenTest{
				workload:          w,
				nemesis:           n,
				timeLimit:         *timeLimit,
				concurrency:       *concurrency,
				rebalanceInterval: *rebalanceInterval,
				localBinary:       *localBinary,
				nodes:             *nodes,
				skew:              *skew,
				testCount:         *testCount,
			})
			tcEnd(pass)
		}
	}
}
