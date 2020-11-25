/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"golang.org/x/tools/go/packages"
)

var (
	ctxb       = context.Background()
	oc         = &outputCatcher{}
	procId     int
	isTeamcity bool
	testId     int32

	baseDir = pflag.StringP("base", "", "../",
		"Base dir for Dgraph")
	runPkg = pflag.StringP("pkg", "p", "",
		"Only run tests for this package")
	runTest = pflag.StringP("test", "t", "",
		"Only run this test")
	runCustom = pflag.BoolP("custom-only", "o", false,
		"Run only custom cluster tests.")
	count = pflag.IntP("count", "c", 0,
		"If set, would add -count arg to go test.")
	concurrency = pflag.IntP("concurrency", "j", 3,
		"Number of clusters to run concurrently.")
	keepCluster = pflag.BoolP("keep", "k", false,
		"Keep the clusters running on program end.")
	clear = pflag.BoolP("clear", "r", false,
		"Clear all the test clusters.")
	dry = pflag.BoolP("dry", "", false,
		"Just show how the packages would be executed, without running tests.")
	rebuildBinary = pflag.BoolP("rebuild-binary", "", true,
		"Build Dgraph before running tests.")
	useExisting = pflag.String("prefix", "",
		"Don't bring up a cluster, instead use an existing cluster with this prefix.")
)

func commandWithContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd
}

// command takes a list of args and executes them as a program.
// Example:
//   docker-compose up -f "./my docker compose.yml"
// would become:
//   command("docker-compose", "up", "-f", "./my docker compose.yml")
func command(args ...string) *exec.Cmd {
	return commandWithContext(ctxb, args...)
}
func runFatal(cmd *exec.Cmd) {
	if err := cmd.Run(); err != nil {
		log.Fatalf("While running command: %q Error: %v\n",
			strings.Join(cmd.Args, " "), err)
	}
}
func startCluster(composeFile, prefix string) {
	cmd := command(
		"docker-compose", "-f", composeFile, "-p", prefix,
		"up", "--force-recreate", "--remove-orphans", "--detach")
	cmd.Stderr = nil

	fmt.Printf("Bringing up cluster %s...\n", prefix)
	runFatal(cmd)
	fmt.Printf("CLUSTER UP: %s\n", prefix)

	// Wait for cluster to be healthy.
	for i := 1; i <= 6; i++ {
		in := getInstance(prefix, "alpha"+strconv.Itoa(i))
		in.bestEffortWaitForHealthy(8080)
	}
}
func stopCluster(composeFile, prefix string, wg *sync.WaitGroup) {
	go func() {
		cmd := command("docker-compose", "-f", composeFile, "-p", prefix, "down", "-v")
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error while bringing down cluster. Prefix: %s. Error: %v\n",
				prefix, err)
		} else {
			fmt.Printf("CLUSTER DOWN: %s\n", prefix)
		}
		wg.Done()
	}()
}

type instance struct {
	Prefix string
	Name   string
}

func getInstance(prefix, name string) instance {
	return instance{Prefix: prefix, Name: name}
}
func (in instance) String() string {
	return fmt.Sprintf("%s_%s_1", in.Prefix, in.Name)
}

func (in instance) bestEffortWaitForHealthy(privatePort uint16) {
	port := in.publicPort(privatePort)
	if len(port) == 0 {
		return
	}
	for i := 0; i < 30; i++ {
		resp, err := http.Get("http://localhost:" + port + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			return
		}
		var body []byte
		if resp != nil && resp.Body != nil {
			body, _ = ioutil.ReadAll(resp.Body)
			resp.Body.Close()
		}
		fmt.Printf("Health for %s failed: %v. Response: %q. Retrying...\n", in, err, body)
		time.Sleep(time.Second)
	}
	return
}

func (in instance) getContainer() types.Container {
	containers := allContainers(in.Prefix)

	q := fmt.Sprintf("/%s_%s_", in.Prefix, in.Name)
	for _, container := range containers {
		for _, name := range container.Names {
			if strings.HasPrefix(name, q) {
				return container
			}
		}
	}
	return types.Container{}
}

func (in instance) publicPort(privatePort uint16) string {
	c := in.getContainer()
	for _, p := range c.Ports {
		if p.PrivatePort == privatePort {
			return strconv.Itoa(int(p.PublicPort))
		}
	}
	return ""
}

func (in instance) login() error {
	addr := in.publicPort(9080)
	if len(addr) == 0 {
		return fmt.Errorf("unable to find container: %s", in)
	}
	dg, err := testutil.DgraphClientWithGroot("localhost:" + addr)
	if err != nil {
		return fmt.Errorf("while connecting: %v", err)
	}
	ctx, cancel := context.WithTimeout(ctxb, 10*time.Second)
	defer cancel()
	if err := dg.Login(ctx, "groot", "password"); err != nil {
		return fmt.Errorf("while logging in: %v", err)
	}
	fmt.Printf("Logged into %s\n", in)
	return nil
}

func (in instance) loginFatal() {
	for i := 0; i < 30; i++ {
		err := in.login()
		if err == nil {
			return
		}
		fmt.Printf("Login failed: %v. Retrying...\n", err)
		time.Sleep(time.Second)
	}
	glog.Fatalf("Unable to login to %s\n", in)
}

func allContainers(prefix string) []types.Container {
	cli, err := client.NewEnvClient()
	x.Check(err)

	containers, err := cli.ContainerList(ctxb, types.ContainerListOptions{All: true})
	if err != nil {
		log.Fatalf("While listing container: %v\n", err)
	}

	var out []types.Container
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.HasPrefix(name, "/"+prefix) {
				out = append(out, c)
			}
		}
	}
	return out
}

func runTestsFor(ctx context.Context, pkg, prefix string) error {
	var args = []string{"go", "test", "-failfast", "-v"}
	if *count > 0 {
		args = append(args, "-count="+strconv.Itoa(*count))
	}
	if len(*runTest) > 0 {
		args = append(args, "-run="+*runTest)
	}
	if isTeamcity {
		args = append(args, "-json")
	}
	args = append(args, pkg)
	cmd := commandWithContext(ctx, args...)
	cmd.Env = append(cmd.Env, "TEST_DOCKER_PREFIX="+prefix)

	// Use failureCatcher.
	cmd.Stdout = oc

	fmt.Printf("Running: %s with %s\n", cmd, prefix)
	start := time.Now()

	if *dry {
		time.Sleep(time.Second)
	} else {
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("While running command: %v Error: %v", args, err)
		}
	}

	dur := time.Since(start).Round(time.Second)
	tid, _ := ctx.Value("threadId").(int32)
	oc.Took(tid, pkg, dur)
	fmt.Printf("Ran tests for package: %s in %s\n", pkg, dur)
	return nil
}

func hasTestFiles(pkg string) bool {
	dir := strings.Replace(pkg, "github.com/dgraph-io/dgraph/", "", 1)
	dir = path.Join(*baseDir, dir)

	hasTests := false
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if hasTests {
			return filepath.SkipDir
		}
		if strings.HasSuffix(path, "_test.go") {
			hasTests = true
			return filepath.SkipDir
		}
		return nil
	})
	return hasTests
}

var _threadId int32

func runTests(taskCh chan task, closer *z.Closer) error {
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

	defaultCompose := path.Join(*baseDir, "dgraph/docker-compose.yml")
	prefix := getPrefix()

	var started, stopped bool
	start := func() {
		if len(*useExisting) > 0 || started {
			return
		}
		startCluster(defaultCompose, prefix)
		started = true

		// Wait for cluster to be healthy.
		getInstance(prefix, "alpha1").loginFatal()
	}

	stop := func() {
		if *keepCluster || stopped {
			return
		}
		wg.Add(1)
		stopCluster(defaultCompose, prefix, wg)
		stopped = true
	}
	defer stop()

	ctx := closer.Ctx()
	ctx = context.WithValue(ctx, "threadId", threadId)

	for task := range taskCh {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !hasTestFiles(task.pkg.ID) {
			continue
		}

		if task.isCommon {
			if *runCustom {
				// If we only need to run custom cluster tests, then skip this one.
				continue
			}
			start()
			if err := runTestsFor(ctx, task.pkg.ID, prefix); err != nil {
				return err
			}
		} else {
			if err := runCustomClusterTest(ctx, task.pkg.ID, wg); err != nil {
				return err
			}
		}
	}
	return nil
}

func getPrefix() string {
	if len(*useExisting) > 0 {
		return *useExisting
	}
	id := atomic.AddInt32(&testId, 1)
	return fmt.Sprintf("test-%03d-%d", procId, id)
}

func runCustomClusterTest(ctx context.Context, pkg string, wg *sync.WaitGroup) error {
	fmt.Printf("Bringing up cluster for package: %s\n", pkg)

	compose := composeFileFor(pkg)
	prefix := getPrefix()

	startCluster(compose, prefix)
	if !*keepCluster {
		wg.Add(1)
		defer stopCluster(compose, prefix, wg)
	}

	return runTestsFor(ctx, pkg, prefix)
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
			dir := strings.Replace(path.Dir(fname), *baseDir, "", 1)
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

	if bytes.Index(p, []byte("FAIL")) >= 0 ||
		bytes.Index(p, []byte("TODO")) >= 0 {
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

func composeFileFor(pkg string) string {
	dir := strings.Replace(pkg, "github.com/dgraph-io/dgraph/", "", 1)
	return path.Join(*baseDir, dir, "docker-compose.yml")
}

func getPackages() []task {
	has := func(list []string, in string) bool {
		for _, l := range list {
			if strings.Contains(in, l) {
				return true
			}
		}
		return false
	}

	moveSlowToFront := func(list []task) {
		slowPkgs := []string{"systest", "ee/acl", "cmd/alpha", "worker"}
		left := 0
		for i := 0; i < len(list); i++ {
			// These packages take time. So, move them to the front.
			if has(slowPkgs, list[i].pkg.ID) {
				list[left], list[i] = list[i], list[left]
				left++
			}
		}
	}

	pkgs, err := packages.Load(nil, *baseDir+"/...")
	x.Check(err)
	limitTo := findPackagesFor(*runTest)

	var valid []task
	for _, pkg := range pkgs {
		if len(*runPkg) > 0 && !strings.HasSuffix(pkg.ID, *runPkg) {
			continue
		}

		if len(*runTest) > 0 {
			if !has(limitTo, pkg.ID) {
				continue
			}
			fmt.Printf("Found package for %s: %s\n", *runTest, pkg.ID)
		}

		fname := composeFileFor(pkg.ID)
		_, err := os.Stat(fname)
		t := task{pkg: pkg, isCommon: os.IsNotExist(err)}
		valid = append(valid, t)
	}
	moveSlowToFront(valid)
	if len(valid) == 0 {
		fmt.Println("Couldn't find any packages. Exiting...")
		os.Exit(1)
	}
	for _, task := range valid {
		fmt.Printf("Found valid task: %s isCommon:%v\n", task.pkg.ID, task.isCommon)
	}
	fmt.Printf("Running tests for %d packages.\n", len(valid))
	return valid
}

func removeAllTestContainers() {
	containers := allContainers("test-")

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
		if strings.HasPrefix(n.Name, "test-") {
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
		if strings.HasPrefix(v.Name, "test-") {
			if err := cli.VolumeRemove(ctxb, v.Name, true); err != nil {
				fmt.Printf("Error: %v while removing volume: %+v\n", err, v)
			} else {
				fmt.Printf("Removed volume: %s\n", v.Name)
			}
		}
	}
}

func run() error {
	if *clear {
		removeAllTestContainers()
		return nil
	}
	fmt.Printf("Proc ID is %d\n", procId)

	start := time.Now()
	oc.Took(0, "START", time.Millisecond)

	if *rebuildBinary {
		// cmd := command("make", "BUILD_RACE=y", "install")
		cmd := command("make", "install")
		cmd.Dir = *baseDir
		if err := cmd.Run(); err != nil {
			return err
		}
		oc.Took(0, "COMPILE", time.Since(start))
	}

	if len(*runPkg) > 0 && len(*runTest) > 0 {
		log.Fatalf("Both pkg and test can't be set.\n")
	}
	tmpDir, err := ioutil.TempDir("", "dgraph-test")
	x.Check(err)
	defer os.RemoveAll(tmpDir)

	if tc := os.Getenv("TEAMCITY_VERSION"); len(tc) > 0 {
		fmt.Printf("Found Teamcity: %s\n", tc)
		isTeamcity = true
	}

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

func main() {
	pflag.Parse()
	rand.Seed(time.Now().UnixNano())
	procId = rand.Intn(1000)

	err := run()
	if err != nil {
		os.Exit(1)
	}
}
