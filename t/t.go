/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"golang.org/x/tools/go/packages"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

var (
	ctxb               = context.Background()
	oc                 = &outputCatcher{}
	procId             int
	isTeamcity         bool
	testId             int32
	coverageFile       = "coverage.out"
	tmpCoverageFile    = "tmp.out"
	testCovMode        = "atomic"
	coverageFileHeader = fmt.Sprintf("mode: %s", testCovMode)
	testsuite          []string

	baseDir = pflag.StringP("base", "", "../",
		"Base dir for Dgraph")
	runPkg = pflag.StringP("pkg", "p", "",
		"Only run tests for this package")
	runTest = pflag.StringP("test", "t", "",
		"Only run this test")
	testType = pflag.StringP("test-type", "", "",
		"Test type e.g unit,integration,upgrade")
	runCustom = pflag.BoolP("custom-only", "o", false,
		"Run only custom cluster tests.")
	count = pflag.IntP("count", "c", 0,
		"If set, would add -count arg to go test.")
	// formerly 3
	concurrency = pflag.IntP("concurrency", "j", 1,
		"Number of clusters to run concurrently.")
	keepCluster = pflag.BoolP("keep", "k", false,
		"Keep the clusters running on program end.")
	clear = pflag.BoolP("clear", "r", false,
		"Clear all the test clusters.")
	dry = pflag.BoolP("dry", "", false,
		"Just show how the packages would be executed, without running tests.")
	// earlier default was true, want to use binary we build manually
	rebuildBinary = pflag.BoolP("rebuild-binary", "", false,
		"Build Dgraph before running tests.")
	useExisting = pflag.String("prefix", "",
		"Don't bring up a cluster, instead use an existing cluster with this prefix.")
	skipSlow = pflag.BoolP("skip-slow", "s", false,
		"If true, don't run tests on slow packages.")
	suite = pflag.String("suite", "unit", "This flag is used to specify which "+
		"test suites to run. Possible values are all, ldbc, load, unit. Multiple suites can be "+
		"selected like --suite=ldbc,load")
	tmp               = pflag.String("tmp", "", "Temporary directory used to download data.")
	downloadResources = pflag.BoolP("download", "d", true,
		"Flag to specify whether to download resources or not")
	race = pflag.Bool("race", false, "Set true to build with race")
	skip = pflag.String("skip", "",
		"comma separated list of packages that needs to be skipped. "+
			"Package Check uses string.Contains(). Please check the flag carefully")
	runCoverage = pflag.Bool("coverage", false, "Set true to calculate test coverage")
)

func commandWithContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if *runCoverage {
		cmd.Env = append(cmd.Env, "COVERAGE_OUTPUT=--test.coverprofile=coverage.out")
	}
	if runtime.GOARCH == "arm64" {
		cmd.Env = append(cmd.Env, "MINIO_IMAGE_ARCH=RELEASE.2020-11-13T20-10-18Z-arm64")
		cmd.Env = append(cmd.Env, "NFS_SERVER_IMAGE_ARCH=11-arm")
	}

	return cmd
}

// command takes a list of args and executes them as a program.
// Example:
//
//	docker-compose up -f "./my docker compose.yml"
//
// would become:
//
//	command("docker-compose", "up", "-f", "./my docker compose.yml")
func command(args ...string) *exec.Cmd {
	return commandWithContext(ctxb, args...)
}

func startCluster(composeFile, prefix string) error {
	cmd := command(
		"docker-compose", "--compatibility", "-f", composeFile, "-p", prefix,
		"up", "--force-recreate", "--build", "--remove-orphans", "--detach")
	cmd.Stderr = nil

	fmt.Printf("Bringing up cluster %s for package: %s ...\n", prefix, composeFile)
	if err := cmd.Run(); err != nil {
		fmt.Printf("While running command: %q Error: %v\n",
			strings.Join(cmd.Args, " "), err)
		return err
	}
	fmt.Printf("CLUSTER UP: %s. Package: %s\n", prefix, composeFile)

	// Wait for cluster to be healthy.
	for i := 1; i <= 3; i++ {
		in := testutil.GetContainerInstance(prefix, "zero"+strconv.Itoa(i))
		if err := in.BestEffortWaitForHealthy(6080); err != nil {
			fmt.Printf("Error while checking zero health %s. Error %v", in.Name, err)
		}

	}
	for i := 1; i <= 6; i++ {
		in := testutil.GetContainerInstance(prefix, "alpha"+strconv.Itoa(i))
		if err := in.BestEffortWaitForHealthy(8080); err != nil {
			fmt.Printf("Error while checking alpha health %s. Error %v", in.Name, err)
		}
	}
	return nil
}

func detectRace(prefix string) bool {
	if !*race {
		return false
	}
	zeroRaceDetected := testutil.DetectRaceInZeros(prefix)
	alphaRaceDetected := testutil.DetectRaceInAlphas(prefix)
	return zeroRaceDetected || alphaRaceDetected
}

func outputLogs(prefix string) {
	f, err := os.CreateTemp(".", prefix+"*.log")
	x.Check(err)
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("error closing file: %v", err)
		}
	}()
	printLogs := func(container string) {
		in := testutil.GetContainerInstance(prefix, container)
		c := in.GetContainer()
		if c == nil {
			return
		}
		logCmd := exec.Command("docker", "logs", c.ID)
		out, err := logCmd.CombinedOutput()
		x.Check(err)
		if _, err := f.Write(out); err != nil {
			fmt.Printf("error writing container logs to file: %v", err)
		}
		fmt.Printf("Docker logs for %s is %s with error %+v ", c.ID, string(out), err)
	}
	for i := 0; i <= 3; i++ {
		printLogs("zero" + strconv.Itoa(i))
	}

	for i := 0; i <= 6; i++ {
		printLogs("alpha" + strconv.Itoa(i))
	}
	s := fmt.Sprintf("---> LOGS for %s written to %s .\n", prefix, f.Name())
	_, err = oc.Write([]byte(s))
	x.Check(err)
}

func stopCluster(composeFile, prefix string, wg *sync.WaitGroup, err error) {
	go func() {
		if err != nil {
			outputLogs(prefix)
		}
		cmd := command("docker-compose", "--compatibility", "-f", composeFile, "-p", prefix, "stop")
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error while bringing down cluster. Prefix: %s. Error: %v\n",
				prefix, err)
		} else {
			fmt.Printf("CLUSTER STOPPED: %s\n", prefix)
		}

		if *runCoverage {
			// get all matching containers, copy /usr/local/bin/coverage.out
			containers := testutil.AllContainers(prefix)
			for _, c := range containers {
				tmp := fmt.Sprintf("%s.%s", tmpCoverageFile, c.ID)

				containerInfo, err := testutil.DockerInspect(c.ID)
				if err != nil {
					fmt.Printf("error while inspecting container. Prefix: %s. Error: %v\n", prefix, err)
				}

				workDir := containerInfo.Config.WorkingDir

				err = testutil.DockerCpFromContainer(c.ID, workDir+"/coverage.out", tmp)
				if err != nil {
					fmt.Printf("error bringing down cluster. Failed at copying coverage file. Prefix: %s. Error: %v\n",
						prefix, err)
				}

				if err = appendTestCoverageFile(tmp, coverageFile); err != nil {
					fmt.Printf("error bringing down cluster. Failed at appending coverage file. Prefix: %s. Error: %v\n",
						prefix, err,
					)
				}

				_ = os.Remove(tmp)

				coverageBulk := strings.Replace(composeFile, "docker-compose.yml", "coverage_bulk.out", -1)
				if err = appendTestCoverageFile(coverageBulk, coverageFile); err != nil {
					fmt.Printf("Error bringing down cluster. Failed at appending coverage file. Prefix: %s. Error: %v\n",
						prefix, err)
				}
			}
		}

		cmd = command("docker-compose", "--compatibility", "-f", composeFile, "-p", prefix, "down", "-v")
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error while bringing down cluster. Prefix: %s. Error: %v\n",
				prefix, err)
		} else {
			fmt.Printf("CLUSTER AND NETWORK REMOVED: %s\n", prefix)
		}

		wg.Done()
	}()
}

func runTestsFor(ctx context.Context, pkg, prefix string) error {
	var args = []string{"go", "test", "-failfast", "-v"}
	if *race {
		args = append(args, "-timeout", "180m")
		// Todo: There are few race errors in tests itself. Enable this once that is fixed.
		// args = append(args, "-race")
	} else {
		args = append(args, "-timeout", "30m")
	}

	if *count > 0 {
		args = append(args, "-count="+strconv.Itoa(*count))
	}
	if len(*testType) > 0 {
		args = append(args, "-tags="+*testType)
	}
	if len(*runTest) > 0 {
		args = append(args, "-run="+*runTest)
	}
	if isTeamcity {
		args = append(args, "-json")
	}
	if *runCoverage {
		// TODO: this breaks where we parallelize the tests, add coverage support for parallel tests
		args = append(args, fmt.Sprintf("-covermode=%s", testCovMode), fmt.Sprintf("-coverprofile=%s", tmpCoverageFile))
	}
	args = append(args, pkg)
	cmd := commandWithContext(ctx, args...)
	cmd.Env = append(cmd.Env, "TEST_DOCKER_PREFIX="+prefix)
	abs, err := filepath.Abs(*tmp)
	if err != nil {
		return fmt.Errorf("while getting absolute path of tmp directory: %v Error: %v", *tmp, err)
	}
	cmd.Env = append(cmd.Env, "TEST_DATA_DIRECTORY="+abs)
	// Use failureCatcher.
	cmd.Stdout = oc

	fmt.Printf("Running: %s with %s\n", cmd, prefix)
	start := time.Now()

	if *dry {
		time.Sleep(time.Second)
	} else {
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("while running command: %v, error: %v", args, err)
		}
	}

	dur := time.Since(start).Round(time.Second)
	tid, _ := ctx.Value(_threadIdKey{}).(int32)
	oc.Took(tid, pkg, dur)
	fmt.Printf("Ran tests for package: %s in %s\n", pkg, dur)
	if *runCoverage {
		if err = appendTestCoverageFile(tmpCoverageFile, coverageFile); err != nil {
			return err
		}
	}
	if detectRace(prefix) {
		return fmt.Errorf("race condition detected for test package %s and cluster with prefix"+
			" %s. check logs for more details", pkg, prefix)
	}
	return nil
}

func hasTestFiles(pkg string) bool {
	dir := strings.Replace(pkg, "github.com/dgraph-io/dgraph/", "", 1)
	dir = filepath.Join(*baseDir, dir)

	hasTests := false
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if hasTests {
			return filepath.SkipDir
		}
		if strings.HasSuffix(path, "_test.go") {
			hasTests = true
			return filepath.SkipDir
		}
		return nil
	})
	x.Check(err)
	return hasTests
}

type _threadIdKey struct{}

var _threadId int32

func runTests(taskCh chan task, closer *z.Closer) error {
	var err error
	threadId := atomic.AddInt32(&_threadId, 1)

	{
		ts := time.Now()
		defer func() {
			oc.Took(threadId, "DONE", time.Since(ts))
		}()
	}

	wg := new(sync.WaitGroup)
	defer func() {
		wg.Wait()
		closer.Done()
	}()

	defaultCompose := filepath.Join(*baseDir, "dgraph/docker-compose.yml")
	prefix := getClusterPrefix()

	var started, stopped bool
	start := func() {
		if len(*useExisting) > 0 || started {
			return
		}
		err := startCluster(defaultCompose, prefix)
		if err != nil {
			closer.Signal()
		}
		started = true
	}

	stop := func() {
		if *keepCluster || stopped {
			return
		}
		wg.Add(1)
		stopCluster(defaultCompose, prefix, wg, err)
		stopped = true
	}
	defer stop()

	ctx := closer.Ctx()
	ctx = context.WithValue(ctx, _threadIdKey{}, threadId)

	for task := range taskCh {
		if ctx.Err() != nil {
			err = ctx.Err()
			return err
		}
		if !hasTestFiles(task.pkg.ID) {
			continue
		}

		if task.isCommon {
			if *runCustom {
				// If we only need to run custom cluster tests, then skip this one.
				continue
			}
			if strings.Contains(*testType, "integration") {
				start()
			}

			if err = runTestsFor(ctx, task.pkg.ID, prefix); err != nil {
				// fmt.Printf("ERROR for package: %s. Err: %v\n", task.pkg.ID, err)
				return err
			}
		} else {
			// we are not using err variable here because we dont want to
			// print logs of default cluster in case of custom test fail.
			if cerr := runCustomClusterTest(ctx, task.pkg.ID, wg); cerr != nil {
				return cerr
			}
		}
	}
	return err
}

func getGlobalPrefix() string {
	var tc string
	if isTeamcity {
		tc = "tc-"
	}
	return "test-" + tc
}

func getClusterPrefix() string {
	if len(*useExisting) > 0 {
		return *useExisting
	}
	id := atomic.AddInt32(&testId, 1)
	return fmt.Sprintf("%s%03d-%d", getGlobalPrefix(), procId, id)
}

// for tests that require custom docker-compose file (located in test directory)
func runCustomClusterTest(ctx context.Context, pkg string, wg *sync.WaitGroup) error {
	fmt.Printf("Bringing up cluster for package: %s\n", pkg)
	var err error
	compose := composeFileFor(pkg)
	prefix := getClusterPrefix()
	if strings.Contains(*testType, "integration") {
		err = startCluster(compose, prefix)
	}

	if err != nil {
		return err
	}
	if !*keepCluster {
		wg.Add(1)
		defer stopCluster(compose, prefix, wg, err)
	}

	err = runTestsFor(ctx, pkg, prefix)
	return err
}

func findPackagesFor(testName string) []string {
	if len(testName) == 0 {
		return []string{}
	}

	cmd := command("ack", testName, *baseDir, "-l")
	var b bytes.Buffer
	cmd.Stdout = &b
	if err := cmd.Run(); err != nil {
		fmt.Printf("Unable to find %s: %v\n", *runTest, err)
		return []string{}
	}

	var dirs []string
	scan := bufio.NewScanner(&b)
	for scan.Scan() {
		fname := scan.Text()
		if strings.HasSuffix(fname, "_test.go") {
			dir := strings.Replace(filepath.Dir(fname), *baseDir, "", 1)
			dirs = append(dirs, dir)
		}
	}
	fmt.Printf("dirs: %+v\n", dirs)
	return dirs
}

type pkgDuration struct {
	threadId int32
	pkg      string
	dur      time.Duration
	ts       time.Time
}

type outputCatcher struct {
	sync.Mutex
	failure bytes.Buffer
	durs    []pkgDuration
}

func (o *outputCatcher) Took(threadId int32, pkg string, dur time.Duration) {
	o.Lock()
	defer o.Unlock()
	o.durs = append(o.durs, pkgDuration{threadId: threadId, pkg: pkg, dur: dur, ts: time.Now()})
}

func (o *outputCatcher) Write(p []byte) (n int, err error) {
	o.Lock()
	defer o.Unlock()

	if bytes.Contains(p, []byte("FAIL")) ||
		bytes.Contains(p, []byte("TODO")) {
		o.failure.Write(p)
	}
	return os.Stdout.Write(p)
}

func (o *outputCatcher) Print() {
	o.Lock()
	defer o.Unlock()

	sort.Slice(o.durs, func(i, j int) bool {
		return o.durs[i].ts.Before(o.durs[j].ts)
	})

	baseTs := o.durs[0].ts
	fmt.Printf("TIMELINE starting at %s\n", baseTs.Format("3:04:05 PM"))
	for _, dur := range o.durs {
		// Don't capture packages which were fast.
		if dur.dur < time.Second {
			continue
		}
		pkg := strings.Replace(dur.pkg, "github.com/dgraph-io/dgraph/", "", 1)
		fmt.Printf("[%6s]%s[%d] %s took: %s\n", dur.ts.Sub(baseTs).Round(time.Second),
			strings.Repeat("   ", int(dur.threadId)), dur.threadId, pkg,
			dur.dur.Round(time.Second))
	}

	if oc.failure.Len() > 0 {
		fmt.Printf("Caught output:\n%s\n", oc.failure.Bytes())
	}
}

type task struct {
	pkg      *packages.Package
	isCommon bool
}

// for custom cluster tests (i.e. those not using default docker-compose.yml)
func composeFileFor(pkg string) string {
	dir := strings.Replace(pkg, "github.com/dgraph-io/dgraph/", "", 1)
	return filepath.Join(*baseDir, dir, "docker-compose.yml")
}

func getPackages() []task {
	has := func(list []string, in string) bool {
		for _, l := range list {
			if len(l) > 0 && strings.Contains(in+"/", "github.com/dgraph-io/dgraph/"+l+"/") {
				return true
			}
		}
		return false
	}

	slowPkgs := []string{"systest", "ee/acl", "cmd/alpha", "worker", "e2e"}
	skipPkgs := strings.Split(*skip, ",")
	runPkgs := strings.Split(*runPkg, ",")

	moveSlowToFront := func(list []task) []task {
		// These packages typically take over a minute to run.
		left := 0
		for i := 0; i < len(list); i++ {
			// These packages take time. So, move them to the front.
			if has(slowPkgs, list[i].pkg.ID) {
				list[left], list[i] = list[i], list[left]
				left++
			}
		}
		if !*skipSlow {
			return list
		}
		out := list[:0]
		for _, t := range list {
			if !has(slowPkgs, t.pkg.ID) {
				out = append(out, t)
			}
		}
		return out
	}

	pkgs, err := packages.Load(nil, *baseDir+"/...")
	x.Check(err)
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			fmt.Printf("Got errors while reading pkg: %s. Error: %+v", pkg.ID, pkg.Errors)
			os.Exit(1)
		}
	}
	limitTo := findPackagesFor(*runTest)

	var valid []task
	for _, pkg := range pkgs {
		if len(*runPkg) > 0 {
			found := false
			for _, eachPkg := range runPkgs {
				if strings.HasSuffix(pkg.ID, eachPkg) {
					found = true
					break
				}
			}
			if !found {
				// pkg did not match any element of runPkg
				continue
			}
		}

		if len(*runTest) > 0 {
			if !has(limitTo, pkg.ID) {
				continue
			}
			fmt.Printf("Found package for %s: %s\n", *runTest, pkg.ID)
		}

		if !isValidPackageForSuite(pkg.ID) {
			fmt.Printf("Skipping package %s as its not valid for the selected suite %+v \n", pkg.ID, testsuite)
			continue
		}

		if has(skipPkgs, pkg.ID) {
			fmt.Printf("Skipping package %s as its available in skip list \n", pkg.ID)
			continue
		}

		fname := composeFileFor(pkg.ID)
		_, err := os.Stat(fname)
		t := task{pkg: pkg, isCommon: os.IsNotExist(err)}
		valid = append(valid, t)
	}
	valid = moveSlowToFront(valid)
	if len(valid) == 0 {
		fmt.Println("Couldn't find any packages. Exiting...")
		os.Exit(0) // this should not have a non-zero exit code; we should be able to skip all folders & exit-0
	}
	for _, task := range valid {
		fmt.Printf("Found valid task: %s isCommon:%v\n", task.pkg.ID, task.isCommon)
	}
	fmt.Printf("Running tests for %d packages.\n", len(valid))
	return valid
}

func removeAllTestContainers() {
	containers := testutil.AllContainers(getGlobalPrefix())

	cli, err := client.NewEnvClient()
	x.Check(err)
	dur := 10 * time.Second

	var wg sync.WaitGroup
	for _, c := range containers {
		wg.Add(1)
		go func(c types.Container) {
			defer wg.Done()
			err := cli.ContainerStop(ctxb, c.ID, &dur)
			fmt.Printf("Stopped container %s with error: %v\n", c.Names[0], err)

			err = cli.ContainerRemove(ctxb, c.ID, types.ContainerRemoveOptions{})
			fmt.Printf("Removed container %s with error: %v\n", c.Names[0], err)
		}(c)
	}
	wg.Wait()

	networks, err := cli.NetworkList(ctxb, types.NetworkListOptions{})
	x.Check(err)
	for _, n := range networks {
		if strings.HasPrefix(n.Name, getGlobalPrefix()) {
			if err := cli.NetworkRemove(ctxb, n.ID); err != nil {
				fmt.Printf("Error: %v while removing network: %+v\n", err, n)
			} else {
				fmt.Printf("Removed network: %s\n", n.Name)
			}
		}
	}

	volumes, err := cli.VolumeList(ctxb, filters.Args{})
	x.Check(err)
	for _, v := range volumes.Volumes {
		if strings.HasPrefix(v.Name, getGlobalPrefix()) {
			if err := cli.VolumeRemove(ctxb, v.Name, true); err != nil {
				fmt.Printf("Error: %v while removing volume: %+v\n", err, v)
			} else {
				fmt.Printf("Removed volume: %s\n", v.Name)
			}
		}
	}
}

var loadPackages = []string{
	"/systest/21million/bulk",
	"/systest/21million/live",
	"/systest/1million",
	"/systest/bulk_live/bulk",
	"/systest/bulk_live/live",
	"/systest/bgindex",
	"/contrib/scripts",
	"/dgraph/cmd/bulk/systest",
}

func testSuiteContains(suite string) bool {
	for _, str := range testsuite {
		if suite == str {
			return true
		}
	}
	return false
}

func isValidPackageForSuite(pkg string) bool {
	valid := false
	if testSuiteContains("all") {
		valid = true
	}
	if testSuiteContains("ldbc") {
		valid = valid || isLDBCPackage(pkg)
	}
	if testSuiteContains("load") {
		valid = valid || isLoadPackage(pkg)
	}
	if testSuiteContains("unit") {
		valid = valid || (!isLoadPackage(pkg) && !isLDBCPackage(pkg))
	}
	if valid {
		return valid
	}
	return false
}

func isLoadPackage(pkg string) bool {
	for _, p := range loadPackages {
		if strings.HasSuffix(pkg, p) {
			return true
		}
	}
	return false
}

func isLDBCPackage(pkg string) bool {
	return strings.HasSuffix(pkg, "/systest/ldbc")
}

var datafiles = map[string]string{
	"1million-noindex.schema": "https://github.com/dgraph-io/benchmarks/blob/master/data/1million-noindex.schema?raw=true",
	"1million.schema":         "https://github.com/dgraph-io/benchmarks/blob/master/data/1million.schema?raw=true",
	"1million.rdf.gz":         "https://github.com/dgraph-io/benchmarks/blob/master/data/1million.rdf.gz?raw=true",
	"21million.schema":        "https://github.com/dgraph-io/benchmarks/blob/master/data/21million.schema?raw=true",
	"21million.rdf.gz":        "https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz?raw=true",
}

var baseUrl = "https://github.com/dgraph-io/benchmarks/blob/master/ldbc/sf0.3/ldbc_rdf_0.3/"
var suffix = "?raw=true"

var rdfFileNames = [...]string{
	"Deltas.rdf",
	"comment_0.rdf",
	"containerOf_0.rdf",
	"forum_0.rdf",
	"hasCreator_0.rdf",
	"hasInterest_0.rdf",
	"hasMember_0.rdf",
	"hasModerator_0.rdf",
	"hasTag_0.rdf",
	"hasType_0.rdf",
	"isLocatedIn_0.rdf",
	"isPartOf_0.rdf",
	"isSubclassOf_0.rdf",
	"knows_0.rdf",
	"likes_0.rdf",
	"organisation_0.rdf",
	"person_0.rdf",
	"place_0.rdf",
	"post_0.rdf",
	"replyOf_0.rdf",
	"studyAt_0.rdf",
	"tag_0.rdf",
	"tagclass_0.rdf",
	"workAt_0.rdf"}

var ldbcDataFiles = map[string]string{
	"ldbcTypes.schema": "https://github.com/dgraph-io/benchmarks/blob/master/ldbc/sf0.3/ldbcTypes.schema?raw=true",
}

func downloadDataFiles() {
	if !*downloadResources {
		fmt.Print("Skipping downloading of resources\n")
		return
	}
	if *tmp == "" {
		*tmp = os.TempDir()
	}
	x.Check(testutil.MakeDirEmpty([]string{*tmp}))
	for fname, link := range datafiles {
		cmd := exec.Command("wget", "-O", fname, link)
		cmd.Dir = *tmp

		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("Error %v", err)
			fmt.Printf("Output %v", out)
		}
	}
}

func downloadLDBCFiles() {
	if !*downloadResources {
		fmt.Print("Skipping downloading of resources\n")
		return
	}
	if *tmp == "" {
		*tmp = os.TempDir() + "/ldbcData"
	}

	x.Check(testutil.MakeDirEmpty([]string{*tmp}))

	for _, name := range rdfFileNames {
		filepath := baseUrl + name + suffix
		ldbcDataFiles[name] = filepath
	}

	start := time.Now()
	var wg sync.WaitGroup
	for fname, link := range ldbcDataFiles {
		wg.Add(1)
		go func(fname, link string, wg *sync.WaitGroup) {
			defer wg.Done()
			start := time.Now()
			cmd := exec.Command("wget", "-O", fname, link)
			cmd.Dir = *tmp
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Printf("Error %v", err)
				fmt.Printf("Output %v", out)
			}
			fmt.Printf("Downloaded %s to %s in %s \n", fname, *tmp, time.Since(start))
		}(fname, link, &wg)
	}
	wg.Wait()
	fmt.Printf("Downloaded %d files in %s \n", len(ldbcDataFiles), time.Since(start))
}

func createTestCoverageFile(path string) error {
	outFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := outFile.Close(); err != nil {
			glog.Warningf("error closing file: %v", err)
		}
	}()

	cmd := command("echo", coverageFileHeader)
	cmd.Stdout = outFile
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode().IsRegular()
}

// Checks whether the test coverage file generated by go test is empty.
// Empty coverage file are those that only has one line ("mode: <coverage_mode>") and nothing else.
func isTestCoverageEmpty(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return true, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			glog.Warningf("error closing file: %v", err)
		}
	}()

	var l int
	scanner := bufio.NewScanner(file)
	for l = 0; scanner.Scan() && l <= 1; l++ {
	}

	if err = scanner.Err(); err != nil {
		return true, err
	}

	return l <= 1, nil
}

func appendTestCoverageFile(src, des string) error {
	if !fileExists(src) {
		fmt.Printf("src: %s does not exist, skipping file\n", src)
		return nil
	}

	isEmpty, err := isTestCoverageEmpty(src)
	if err != nil {
		return err
	}
	if isEmpty {
		fmt.Printf("no test files or no test coverage statement generated for %s, skipping file\n", src)
		return nil
	}

	cmd := command("bash", "-c", fmt.Sprintf("cat %s | grep -v \"%s\" >> %s", src, coverageFileHeader, des))
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func executePreRunSteps() error {
	testutil.GeneratePlugins(*race)
	if *runCoverage {
		if err := createTestCoverageFile(coverageFile); err != nil {
			return err
		}
	}
	return nil
}

func run() error {
	if tc := os.Getenv("TEAMCITY_VERSION"); len(tc) > 0 {
		fmt.Printf("Found Teamcity: %s\n", tc)
		isTeamcity = true
	}
	if *clear {
		removeAllTestContainers()
		return nil
	}
	if len(*runPkg) > 0 && len(*runTest) > 0 {
		log.Fatalf("Both pkg and test can't be set.\n")
	}
	fmt.Printf("Proc ID is %d\n", procId)
	fmt.Printf("Detected architecture: %s", runtime.GOARCH)

	start := time.Now()
	oc.Took(0, "START", time.Millisecond)

	if *rebuildBinary {
		var cmd *exec.Cmd
		if *race {
			cmd = command("make", "BUILD_RACE=y", "install")
		} else {
			cmd = command("make", "install")
		}
		cmd.Dir = *baseDir
		if err := cmd.Run(); err != nil {
			return err
		}
		oc.Took(0, "COMPILE", time.Since(start))
	}

	tmpDir, err := os.MkdirTemp("", "dgraph-test")
	x.Check(err)
	defer os.RemoveAll(tmpDir)

	err = executePreRunSteps()
	x.Check(err)
	N := *concurrency
	if len(*runPkg) > 0 || len(*runTest) > 0 {
		N = 1
	}
	closer := z.NewCloser(N)
	testCh := make(chan task)
	errCh := make(chan error, 1000)
	for i := 0; i < N; i++ {
		go func() {
			if err := runTests(testCh, closer); err != nil {
				errCh <- err
				closer.Signal()
			}
		}()
	}

	sdCh := make(chan os.Signal, 3)
	defer func() {
		signal.Stop(sdCh)
		close(sdCh)
	}()
	go func() {
		var count int
		for range sdCh {
			count++
			if count == 3 {
				os.Exit(1)
			}
			closer.Signal()
		}
	}()
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// pkgs, err := packages.Load(nil, "github.com/dgraph-io/dgraph/...")
	go func() {
		defer close(testCh)
		valid := getPackages()
		if testSuiteContains("load") || testSuiteContains("all") {
			downloadDataFiles()
		}
		if testSuiteContains("ldbc") || testSuiteContains("all") {
			downloadLDBCFiles()
		}
		for i, task := range valid {
			select {
			case testCh <- task:
				fmt.Printf("Sent %d/%d packages for processing.\n", i+1, len(valid))
			case <-closer.HasBeenClosed():
				return
			}
		}
	}()

	closer.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			oc.Print()
			fmt.Printf("Got error: %v.\n", err)
			fmt.Println("Tests FAILED.")
			return err
		}
	}
	oc.Print()
	fmt.Printf("Tests PASSED. Time taken: %v\n", time.Since(start).Truncate(time.Second))
	return nil
}

func validateAllowed(testSuite []string) {

	allowed := []string{"all", "ldbc", "load", "unit"}
	for _, str := range testSuite {
		onlyAllowed := false
		for _, allowedStr := range allowed {
			if str == allowedStr {
				onlyAllowed = true
			}
		}
		if !onlyAllowed {
			log.Fatalf("Allowed options for suite are only all, load, ldbc or unit; passed in %+v", testSuite)
		}
	}
}

func main() {
	pflag.Parse()
	testsuite = strings.Split(*suite, ",")
	validateAllowed(testsuite)

	rand.Seed(time.Now().UnixNano())
	procId = rand.Intn(1000)

	err := run()
	_ = os.RemoveAll(*tmp)
	if err != nil {
		os.Exit(1)
	}
}
