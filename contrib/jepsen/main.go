/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/pflag"

	"github.com/dgraph-io/dgraph/contrib/jepsen/browser"
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
	replicas          int
	skew              string
	testCount         int
	deferDbTeardown   bool
}

var (
	errTestFail       = errors.New("test failed")
	errTestIncomplete = errors.New("test incomplete")
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
	replicas  = pflag.Int("replicas", 3, "How many replicas of data should dgraph store?")
	skew      = pflag.String("skew", "", "Skew clock amount. (tiny, small, big, huge)")
	testCount = pflag.IntP("test-count", "c", 1, "Test count per Jepsen test.")
	jaeger    = pflag.StringP("jaeger", "j", "",
		"Run with Jaeger collector. Set to empty string to disable collection to Jaeger."+
			" Otherwise set to http://jaeger:14268.")
	jaegerSaveTraces = pflag.Bool("jaeger-save-traces", true, "Save Jaeger traces on test error.")
	deferDbTeardown  = pflag.Bool("defer-db-teardown", false,
		"Wait until user input to tear down DB nodes")

	// Jepsen control flags
	doUp       = pflag.BoolP("up", "u", true, "Run Jepsen ./up.sh.")
	doUpOnly   = pflag.BoolP("up-only", "U", false, "Do --up and exit.")
	doDown     = pflag.BoolP("down", "d", false, "Stop the Jepsen cluster after tests run.")
	doDownOnly = pflag.BoolP("down-only", "D", false, "Do --down and exit. Does not run tests.")
	web        = pflag.Bool("web", true, "Open the test results page in the browser.")

	// Option to run each test with a new cluster. This appears to mitigate flakiness.
	refreshCluster = pflag.Bool("refresh-cluster", false,
		"Down and up the cluster before each test.")

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
	exitOnFailure = pflag.BoolP("exit-on-failure", "e", false,
		"Don't run any more tests after a failure.")
)

const (
	maxRetries = 5
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

func jepsenUp(jepsenPath string) {
	cmd := command("./up.sh", "--dev", "--daemon",
		"--compose", "../dgraph/docker/docker-compose.yml")
	cmd.Dir = jepsenPath + "/docker/"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	env := os.Environ()
	cmd.Env = append(env, fmt.Sprintf("JEPSEN_ROOT=%s", *jepsenRoot))
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func jepsenDown(jepsenPath string) {
	cmd := command("docker-compose",
		"-f", "./docker-compose.yml",
		"-f", "../dgraph/docker/docker-compose.yml",
		"down")
	cmd.Dir = jepsenPath + "/docker/"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		switch {
		case strings.Contains(err.Error(), "Couldn't find env file"):
			// This is OK. Probably tried to call down before up was ever called.
		default:
			log.Println(err)
		}
	}
}

func jepsenServe() error {
	// Check if the page is already up
	checkServing := func() error {
		url := jepsenURL()
		_, err := http.Get(url) // nolint:gosec
		return err
	}
	if err := checkServing(); err == nil {
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	errCh := make(chan error)
	go func() {
		// If this runs for the first time it takes about a minute before
		// starting in order to fetch and install dependencies.
		cmd := command(
			"docker", "exec", "--workdir", "/jepsen/dgraph", "jepsen-control",
			"lein", "run", "serve")
		if *dryRun {
			wg.Done()
			errCh <- nil
			return
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout
		// lein run serve runs indefinitely, so there's no need to wait for the
		// command to finish.
		_ = cmd.Start()
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-time.After(5 * time.Minute):
				wg.Done()
				errCh <- errors.New("lein run serve couldn't run after 5 minutes")
				return
			case <-ticker.C:
				if err := checkServing(); err == nil {
					ticker.Stop()
					wg.Done()
					errCh <- nil
					return
				}
			default:
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	wg.Wait()
	return <-errCh
}

func jepsenURL() string {
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
	return "http://localhost:" + port
}

func runJepsenTest(test *jepsenTest) error {
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
		"--replicas", strconv.Itoa(test.replicas),
		"--test-count", strconv.Itoa(test.testCount),
	}
	if test.nemesis == "skew-clock" {
		testCmd = append(testCmd, "--skew", test.skew)
	}
	if *jaeger != "" {
		testCmd = append(testCmd,
			"--dgraph-jaeger-collector", *jaeger,
			"--tracing", *jaeger+"/api/traces")
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
			return errTestFail
		}
		return errTestIncomplete
	}
	if strings.Contains(out.String(), "Everything looks good!") {
		return nil
	}
	return errTestIncomplete
}

func inCi() bool {
	return *ciOutput || os.Getenv("TEAMCITY_VERSION") != ""
}

func saveJaegerTracesToJepsen(jepsenPath string) {
	dst := filepath.Join(jepsenPath, "dgraph", "store", "current", "jaeger")
	cmd := command("sudo", "docker", "cp", "jaeger:/working/jaeger", dst)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
	absDst, err := os.Readlink(dst)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Saved Jaeger traces to %v\n", absDst)
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

	shouldOpenPage := *web && !*dryRun

	if *doDownOnly {
		jepsenDown(*jepsenRoot)
		os.Exit(0)
	}
	if *doUpOnly {
		jepsenUp(*jepsenRoot)
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

	if *doDown && !*refreshCluster {
		jepsenDown(*jepsenRoot)
	}
	if *doUp && !*refreshCluster {
		jepsenUp(*jepsenRoot)
	}

	if !*refreshCluster {
		if err := jepsenServe(); err != nil {
			log.Fatal(err)
		}
		if shouldOpenPage {
			url := jepsenURL()
			browser.Open(url)
			if *jaeger != "" {
				browser.Open("http://localhost:16686")
			}
		}
	}

	workloads := strings.Split(*workload, " ")
	nemeses := strings.Split(*nemesis, " ")
	fmt.Printf("Num tests: %v\n", len(workloads)*len(nemeses))
	for _, n := range nemeses {
		for _, w := range workloads {
			tries := 0
		retryLoop:
			for {
				if *refreshCluster {
					jepsenDown(*jepsenRoot)
					jepsenUp(*jepsenRoot)
					if err := jepsenServe(); err != nil {
						log.Fatal(err)
					}
					// Sleep for 10 seconds to let the cluster start before running the test.
					time.Sleep(10 * time.Second)
				}

				err := runJepsenTest(&jepsenTest{
					workload:          w,
					nemesis:           n,
					timeLimit:         *timeLimit,
					concurrency:       *concurrency,
					rebalanceInterval: *rebalanceInterval,
					nemesisInterval:   *nemesisInterval,
					localBinary:       *localBinary,
					nodes:             *nodes,
					replicas:          *replicas,
					skew:              *skew,
					testCount:         *testCount,
					deferDbTeardown:   *deferDbTeardown,
				})

				switch err {
				case nil:
					break retryLoop
				case errTestFail:
					if *jaegerSaveTraces {
						saveJaegerTracesToJepsen(*jepsenRoot)
					}
					if *exitOnFailure {
						os.Exit(1)
					}
					defer os.Exit(1)
					break retryLoop
				case errTestIncomplete:
					// Retry incomplete tests. Sometimes tests fail due to temporary errors.
					tries++
					if tries == maxRetries {
						fmt.Fprintf(os.Stderr, "Test with workload %s and nemesis %s could not "+
							"start after maximum number of retries", w, n)
						defer os.Exit(1)
						break retryLoop
					} else {
						continue
					}
				}
			}
		}
	}
}
