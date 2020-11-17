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
	"github.com/docker/docker/client"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"golang.org/x/tools/go/packages"
)

var (
	ctxb       = context.Background()
	fc         = &failureCatcher{}
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
	concurrency = pflag.IntP("concurrency", "j", 1,
		"Number of clusters to run concurrently. There's a bug somewhere causing"+
			" tests to fail on any concurrency setting > 1.")
	keepCluster = pflag.BoolP("keep", "k", false,
		"Keep the clusters running on program end.")
)

func commandWithContext(ctx context.Context, q string) *exec.Cmd {
	splits := strings.Split(q, " ")
	sane := splits[:0]
	for _, s := range splits {
		if s != "" {
			sane = append(sane, s)
		}
	}
	cmd := exec.CommandContext(ctx, sane[0], sane[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd
}
func command(cmd string) *exec.Cmd {
	return commandWithContext(ctxb, cmd)
}
func runFatal(q string) {
	cmd := command(q)
	if err := cmd.Run(); err != nil {
		log.Fatalf("While running command: %q Error: %v\n",
			q, err)
	}
}
func startCluster(composeFile, prefix string) {
	q := fmt.Sprintf("docker-compose -f %s -p %s up --force-recreate --remove-orphans --detach",
		composeFile, prefix)
	runFatal(q)

	// Let it stabilize.
	time.Sleep(3 * time.Second)
}
func stopCluster(composeFile, prefix string, wg *sync.WaitGroup) {
	q := fmt.Sprintf("docker-compose -f %s -p %s down",
		composeFile, prefix)
	go func() {
		runFatal(q)
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

func allContainers(prefix string) []types.Container {
	cli, err := client.NewEnvClient()
	x.Check(err)

	containers, err := cli.ContainerList(ctxb, types.ContainerListOptions{})
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

func runTestsFor(ctx context.Context, pkg, prefix string) error {
	var opts []string
	if *count > 0 {
		opts = append(opts, "-count="+strconv.Itoa(*count))
	}
	if len(*runTest) > 0 {
		opts = append(opts, "-run="+*runTest)
	}
	q := fmt.Sprintf("go test -v %s %s", strings.Join(opts, " "), pkg)
	cmd := commandWithContext(ctx, q)
	cmd.Env = append(cmd.Env, "TEST_DOCKER_PREFIX="+prefix)

	// Use failureCatcher.
	cmd.Stdout = fc

	fmt.Printf("Running: %s with %s\n", cmd, prefix)
	start := time.Now()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("While running command: %q Error: %v", q, err)
	}

	dur := time.Since(start).Round(time.Second)
	fc.Took(prefix, pkg, dur)
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

func runTests(taskCh chan task, closer *z.Closer) error {
	wg := new(sync.WaitGroup)
	defer func() {
		wg.Wait()
		closer.Done()
	}()

	defaultCompose := path.Join(*baseDir, "dgraph/docker-compose.yml")
	prefix := getPrefix()

	var started, stopped bool
	start := func() {
		if started {
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

	uncommon := false
	for task := range taskCh {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if uncommon && task.isCommon {
			glog.Fatalf("Package sorting is wrong. Common cluster tests should run first.")
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
			uncommon = true
			stop() // Stop default cluster.

			if err := runCustomClusterTest(ctx, task.pkg.ID, wg); err != nil {
				return err
			}
		}
	}
	return nil
}

func getPrefix() string {
	id := atomic.AddInt32(&testId, 1)
	return fmt.Sprintf("test-%03d-%d", procId, id)
}

func runCustomClusterTest(ctx context.Context, pkg string, wg *sync.WaitGroup) error {
	compose := composeFileFor(pkg)
	prefix := getPrefix()

	startCluster(compose, prefix)
	if !*keepCluster {
		wg.Add(1)
		defer stopCluster(compose, prefix, wg)
	}

	port := getInstance(prefix, "alpha1").publicPort(8080)

	for i := 0; i < 30; i++ {
		resp, err := http.Get("http://localhost:" + port + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			fmt.Printf("Health check: OK for %s. Status: %s\n", prefix, resp.Status)
			break
		}
		var body []byte
		if resp != nil && resp.Body != nil {
			body, _ = ioutil.ReadAll(resp.Body)
			resp.Body.Close()
		}
		fmt.Printf("Health failed: %v. Response: %q. Retrying...\n", err, body)
		time.Sleep(time.Second)
	}

	// Wait for cluster to be healthy.
	// getInstance(prefix, "alpha1").loginFatal()

	return runTestsFor(ctx, pkg, prefix)
}

func findPackagesFor(testName string) []string {
	if len(testName) == 0 {
		return []string{}
	}

	cmd := command(fmt.Sprintf("ack %s %s -l", testName, *baseDir))
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
	prefix string
	pkg    string
	dur    time.Duration
	ts     time.Time
}

type failureCatcher struct {
	sync.Mutex
	failure bytes.Buffer
	durs    []pkgDuration
}

func (o *failureCatcher) Took(prefix, pkg string, dur time.Duration) {
	o.Lock()
	defer o.Unlock()
	o.durs = append(o.durs, pkgDuration{prefix: prefix, pkg: pkg, dur: dur, ts: time.Now()})
}

func (o *failureCatcher) Write(p []byte) (n int, err error) {
	o.Lock()
	defer o.Unlock()

	if bytes.Index(p, []byte("FAIL")) >= 0 {
		o.failure.Write(p)
	}
	return os.Stdout.Write(p)
}

func (o *failureCatcher) Print() {
	o.Lock()
	defer o.Unlock()

	fmt.Println("TIMELINE")
	sort.Slice(o.durs, func(i, j int) bool {
		return o.durs[i].ts.Before(o.durs[j].ts)
	})
	for _, dur := range o.durs {
		// Don't capture packages which were fast.
		if dur.dur < 5*time.Second {
			continue
		}
		fmt.Printf("[%s] [%s] pkg %s took: %s\n", dur.ts.Format(time.Kitchen),
			dur.prefix, dur.pkg, dur.dur)
	}

	// sort.Slice(o.durs, func(i, j int) bool {
	// 	return o.durs[i].dur > o.durs[j].dur
	// })
	// for _, dur := range o.durs {
	// 	if dur > 10*time.Second {
	// 		continue
	// 	}
	// 	fmt.Printf("Took: %s Package: %s\n", dur.dur, dur.pkg)
	// }
	if fc.failure.Len() > 0 {
		fmt.Printf("Failure output: %s\n", fc.failure.Bytes())
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
	pkgs, err := packages.Load(nil, *baseDir+"/...")
	x.Check(err)

	has := func(list []string, in string) bool {
		for _, l := range list {
			if strings.Contains(in, l) {
				return true
			}
		}
		return false
	}

	slowPkgs := []string{"systest", "ee/acl", "cmd/alpha"}
	left := 0
	for i := 0; i < len(pkgs); i++ {
		// These packages take time. So, move them to the front.
		if has(slowPkgs, pkgs[i].ID) {
			pkgs[left], pkgs[i] = pkgs[i], pkgs[left]
			left++
			break
		}
	}

	limitTo := findPackagesFor(*runTest)

	var valid []task
	for _, pkg := range pkgs {
		if len(*runPkg) > 0 && !strings.HasSuffix(pkg.ID, *runPkg) {
			continue
		}
		if has([]string{"mtls_internal", "graphql"}, pkg.ID) {
			fmt.Printf("SKIPPING tests for package: %s for now. PLEASE FIX ASAP.\n", pkg.ID)
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
		valid = append(valid, task{pkg: pkg, isCommon: os.IsNotExist(err)})
	}

	if len(valid) == 0 {
		fmt.Println("Couldn't find any packages. Exiting...")
		os.Exit(1)
	}

	sort.SliceStable(valid, func(i, j int) bool {
		if valid[i].isCommon != valid[j].isCommon {
			return valid[i].isCommon
		}
		return false
	})

	for _, task := range valid {
		fmt.Printf("Found valid task: %s isCommon:%v\n", task.pkg.ID, task.isCommon)
	}
	fmt.Printf("Running tests for %d packages.\n", len(valid))
	return valid
}

func main() {
	pflag.Parse()
	rand.Seed(time.Now().UnixNano())
	procId = rand.Intn(1000)
	start := time.Now()

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
		for {
			select {
			case <-sdCh:
				count++
				if count == 3 {
					os.Exit(1)
				}
				closer.Signal()
			}
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
			fc.Print()
			fmt.Printf("Got error: %v.\n", err)
			fmt.Println("Tests FAILED.")
			os.Exit(1)
		}
	}
	fc.Print()
	fmt.Printf("Tests PASSED. Time taken: %v\n", time.Since(start).Truncate(time.Second))
}
