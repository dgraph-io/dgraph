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
// Set the --jepsen-root flag to the path of the Jepsen repo directory.
//
// Example usage:
//
// Runs all test and nemesis combinations (36 total)
//     ./jepsen --jepsen-root $JEPSEN_ROOT --test-all
//
// Runs bank test with partition-ring nemesis for 10 minutes
//     ./jepsen --jepsen-root $JEPSEN_ROOT --workload bank --nemesis partition-ring

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/contrib/jepsen/browser"
	"github.com/spf13/pflag"
)

type jepsenTest struct {
	workload          string
	nemesis           string
	timeLimit         int
	concurrency       string
	rebalanceInterval string
	nemesisInterval   string
	localBinary       string
	nodes             string
	skew              string
	testCount         int
}

const (
	testPass = iota
	testFail
	testIncomplete
)

var (
	availableWorkloads = []string{
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
	availableNemeses = []string{
		"none",
		"kill-alpha",
		"kill-zero",
		"partition-ring",
		"move-tablet",
	}

	testAllWorkloads = availableWorkloads
	testAllNemeses   = []string{
		"none",
		// the kill nemeses run together
		"kill-alpha,kill-zero",
		"partition-ring",
		"move-tablet",
	}
)

var (
	ctxb = context.Background()

	// Jepsen test flags
	workload = pflag.StringP("workload", "w", "",
		"Test workload to run. Specify a space-separated list of workloads. Available workloads: "+
			fmt.Sprintf("%q", availableWorkloads))
	nemesis = pflag.StringP("nemesis", "n", "",
		"A space-separated, comma-separated list of nemesis types. "+
			"Combinations of nemeses can be specified by combining them with commas, "+
			"e.g., kill-alpha,kill-zero. Available nemeses: "+
			fmt.Sprintf("%q", availableNemeses))
	timeLimit = pflag.IntP("time-limit", "l", 600,
		"Time limit per Jepsen test in seconds.")
	concurrency = pflag.String("concurrency", "6n",
		"Number of concurrent workers per test. \"6n\" means 6 workers per node.")
	rebalanceInterval = pflag.String("rebalance-interval", "10h",
		"Interval of Dgraph's tablet rebalancing.")
	nemesisInterval = pflag.String("nemesis-interval", "10",
		"Roughly how long to wait (in seconds) between nemesis operations.")
	localBinary = pflag.StringP("local-binary", "b", "/gobin/dgraph",
		"Path to Dgraph binary within the Jepsen control node.")
	nodes     = pflag.String("nodes", "n1,n2,n3,n4,n5", "Nodes to run on.")
	skew      = pflag.String("skew", "", "Skew clock amount. (tiny, small, big, huge)")
	testCount = pflag.IntP("test-count", "c", 1, "Test count per Jepsen test.")
	jaeger    = pflag.StringP("jaeger", "j", "http://jaeger:14268",
		"Run with Jaeger collector. Set to empty string to disable collection to Jaeger.")

	// Jepsen control flags
	doUp       = pflag.BoolP("up", "u", true, "Run Jepsen ./up.sh.")
	doUpOnly   = pflag.BoolP("up-only", "U", false, "Do --up and exit.")
	doDown     = pflag.BoolP("down", "d", false, "Stop the Jepsen cluster after tests run.")
	doDownOnly = pflag.BoolP("down-only", "D", false, "Do --down and exit. Does not run tests.")
	doServe    = pflag.Bool("serve", true, "Serve the test results page (lein run serve).")
	web        = pflag.Bool("web", true, "Open the test results page in the browser.")

	// Script flags
	dryRun = pflag.BoolP("dry-run", "y", false,
		"Echo commands that would run, but don't execute them.")
	jepsenRoot = pflag.StringP("jepsen-root", "r", "",
		"Directory path to jepsen repo. This sets the JEPSEN_ROOT env var for Jepsen ./up.sh.")
	ciOutput = pflag.BoolP("ci-output", "q", false,
		"Output TeamCity test result directives instead of Jepsen test output.")
	testAll = pflag.Bool("test-all", false,
		"Run the following workload and nemesis combinations: "+
			fmt.Sprintf("Workloads:%v, Nemeses:%v", testAllWorkloads, testAllNemeses))
)

func command(cmd ...string) *exec.Cmd {
	return commandContext(ctxb, cmd...)
}

func commandContext(ctx context.Context, cmd ...string) *exec.Cmd {
	if *dryRun {
		// Properly quote the args so the echoed output can run via copy/paste.
		quoted := []string{}
		for _, c := range cmd {
			if strings.Contains(c, " ") {
				quoted = append(quoted, strconv.Quote(c))
			} else {
				quoted = append(quoted, c)
			}

		}
		return exec.CommandContext(ctx, "echo", quoted...)
	}
	return exec.CommandContext(ctx, cmd[0], cmd[1:]...)
}

func jepsenUp() {
	cmd := command("./up.sh",
		"--dev", "--daemon", "--compose", "../dgraph/docker/docker-compose.yml")
	cmd.Dir = *jepsenRoot + "/docker/"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	env := os.Environ()
	cmd.Env = append(env, fmt.Sprintf("JEPSEN_ROOT=%s", *jepsenRoot))
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func jepsenDown() {
	cmd := command("docker-compose",
		"-f", "./docker-compose.yml",
		"-f", "../dgraph/docker/docker-compose.yml",
		"down")
	cmd.Dir = *jepsenRoot + "/docker/"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func jepsenServe() {
	cmd := command(
		"docker", "exec", "--workdir", "/jepsen/dgraph", "jepsen-control",
		"lein", "run", "serve")
	// Ignore output and errors. It's okay if "lein run serve" already ran before.
	_ = cmd.Run()
}

func openJepsenBrowser() {
	cmd := command(
		"docker", "inspect", "--format",
		`{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostPort }}`,
		"jepsen-control")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
	port := strings.TrimSpace(out.String())
	jepsenUrl := "http://localhost:" + port
	browser.Open(jepsenUrl)
}

func runJepsenTest(test *jepsenTest) int {
	dockerCmd := []string{
		"docker", "exec", "jepsen-control",
		"/bin/bash", "-c",
	}
	testCmd := []string{
		// setup commands needed to set up ssh-agent to ssh into nodes.
		"source", "~/.bashrc", "&&",
		"cd", "/jepsen/dgraph", "&&",
		// test commands
		"lein", "run", "test",
		"--workload", test.workload,
		"--nemesis", test.nemesis,
		"--time-limit", strconv.Itoa(test.timeLimit),
		"--concurrency", test.concurrency,
		"--rebalance-interval", test.rebalanceInterval,
		"--nemesis-interval", test.nemesisInterval,
		"--local-binary", test.localBinary,
		"--nodes", test.nodes,
		"--test-count", strconv.Itoa(test.testCount),
	}
	if test.nemesis == "skew-clock" {
		testCmd = append(testCmd, "--skew", test.skew)
	}
	if *jaeger != "" {
		testCmd = append(testCmd, "--dgraph-jaeger-collector", *jaeger)
		testCmd = append(testCmd, "--tracing", *jaeger+"/api/traces")
	}
	dockerCmd = append(dockerCmd, strings.Join(testCmd, " "))

	// Timeout should be a bit longer than the Jepsen test time limit to account
	// for post-analysis time.
	commandTimeout := 10*time.Minute + time.Duration(test.timeLimit)*time.Second
	ctx, cancel := context.WithTimeout(ctxb, commandTimeout)
	defer cancel()
	cmd := commandContext(ctx, dockerCmd...)

	var out bytes.Buffer
	var stdout io.Writer
	var stderr io.Writer
	stdout = io.MultiWriter(&out, os.Stdout)
	stderr = io.MultiWriter(&out, os.Stderr)
	if inCi() {
		// Jepsen test output to os.Stdout/os.Stderr is not needed in TeamCity.
		stdout = &out
		stderr = &out
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		// TODO The exit code could probably be checked instead of checking the output.
		// Check jepsen source to be sure.
		if strings.Contains(out.String(), "Analysis invalid") {
			return testFail
		}
		return testIncomplete
	}
	if strings.Contains(out.String(), "Everything looks good!") {
		return testPass
	}
	return testIncomplete
}

func inCi() bool {
	return *ciOutput || os.Getenv("TEAMCITY_VERSION") != ""
}

func tcStart(testName string) func(pass int) {
	if !inCi() {
		return func(int) {}
	}
	now := time.Now()
	fmt.Printf("##teamcity[testStarted name='%v']\n", testName)
	return func(pass int) {
		durMs := time.Since(now).Nanoseconds() / 1e6
		switch pass {
		case testPass:
			fmt.Printf("##teamcity[testFinished name='%v' duration='%v']\n", testName, durMs)
		case testFail:
			fmt.Printf("##teamcity[testFailed='%v' duration='%v']\n", testName, durMs)
		case testIncomplete:
			fmt.Printf("##teamcity[testFailed='%v' duration='%v' message='Test incomplete.']\n",
				testName, durMs)
		}
	}
}

func main() {
	pflag.ErrHelp = errors.New("")
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		pflag.PrintDefaults()

		fmt.Printf("\nExample usage:\n")
		fmt.Printf("$ %v --jepsen-root $JEPSEN_ROOT -w bank -n none\n", os.Args[0])
		fmt.Printf("$ %v --jepsen-root $JEPSEN_ROOT -w 'bank delete' "+
			"-n 'none kill-alpha,kill-zero move-tablet'\n", os.Args[0])
		fmt.Printf("$ %v --jepsen-root $JEPSEN_ROOT --test-all\n", os.Args[0])
	}
	pflag.Parse()

	if *jepsenRoot == "" {
		log.Fatal("--jepsen-root must be set.")
	}
	if os.Getenv("GOPATH") == "" {
		log.Fatal("GOPATH must be set.")
	}

	if *doDownOnly {
		jepsenDown()
		os.Exit(0)
	}
	if *doUpOnly {
		jepsenUp()
		os.Exit(0)
	}

	if *testAll {
		*workload = strings.Join(testAllWorkloads, " ")
		*nemesis = strings.Join(testAllNemeses, " ")
	}

	if *workload == "" || *nemesis == "" {
		fmt.Fprintf(os.Stderr, "You must specify at least one workload and at least one nemesis.\n")
		fmt.Fprintf(os.Stderr, "See --help for example usage.\n")
		os.Exit(1)
	}

	if strings.Contains(*nemesis, "skew-clock") && *skew == "" {
		log.Fatal("skew-clock nemesis specified but --jepsen.skew wasn't set.")
	}

	if *doDown {
		jepsenDown()
	}
	if *doUp {
		jepsenUp()
	}
	if *doServe {
		go jepsenServe()
		if *web && !*dryRun {
			openJepsenBrowser()
		}
	}
	if *web && !*dryRun && *jaeger != "" {
		// Open Jaeger UI
		browser.Open("http://localhost:16686")
	}

	workloads := strings.Split(*workload, " ")
	nemeses := strings.Split(*nemesis, " ")
	fmt.Printf("Num tests: %v\n", len(workloads)*len(nemeses))
	for _, n := range nemeses {
		for _, w := range workloads {
			tcEnd := tcStart(fmt.Sprintf("Workload:%v,Nemeses:%v", w, n))
			status := runJepsenTest(&jepsenTest{
				workload:          w,
				nemesis:           n,
				timeLimit:         *timeLimit,
				concurrency:       *concurrency,
				rebalanceInterval: *rebalanceInterval,
				nemesisInterval:   *nemesisInterval,
				localBinary:       *localBinary,
				nodes:             *nodes,
				skew:              *skew,
				testCount:         *testCount,
			})
			tcEnd(status)
		}
	}
}
