package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strconv"
	"strings"
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
	isTeamcity bool
	testId     int32

	baseDir = pflag.StringP("base", "", "../",
		"Base dir for Dgraph")
	runPkg = pflag.StringP("pkg", "p", "",
		"Only run tests for this package")
	runTest = pflag.StringP("test", "t", "",
		"Only run this test")
	cache = pflag.BoolP("cache", "c", true,
		"If set to false, would force Go to re-run tests.")
	concurrency = pflag.IntP("concurrency", "j", 4,
		"Number of clusters to run concurrently.")
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
func stopCluster(composeFile, prefix string) {
	q := fmt.Sprintf("docker-compose -f %s -p %s down",
		composeFile, prefix)
	runFatal(q)
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
	for i, c := range containers {
		fmt.Printf("[%d] %s\n", i, c.Names[0])
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
	if !*cache {
		opts = append(opts, "-count=1")
	}
	if len(*runTest) > 0 {
		opts = append(opts, "-run="+*runTest)
	}
	q := fmt.Sprintf("go test -v %s %s", strings.Join(opts, " "), pkg)
	cmd := commandWithContext(ctx, q)
	in := instance{
		Prefix: prefix,
		Name:   "alpha" + strconv.Itoa(1+rand.Intn(6)),
	}
	cmd.Env = append(cmd.Env, "TEST_PORT_ALPHA="+in.publicPort(9080))
	cmd.Env = append(cmd.Env, "TEST_PORT_ALPHA_HTTP="+in.publicPort(8080))
	cmd.Env = append(cmd.Env, "TEST_ALPHA="+in.String())

	// in.Name = "alpha1"

	zeroIn := getInstance(prefix, "zero1")
	cmd.Env = append(cmd.Env, "TEST_PORT_ZERO="+zeroIn.publicPort(5080))
	cmd.Env = append(cmd.Env, "TEST_PORT_ZERO_HTTP="+zeroIn.publicPort(6080))

	fmt.Printf("Running: %s with %s\n", cmd, in)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("While running command: %q Error: %v", q, err)
	}
	fmt.Printf("Ran tests for package: %s\n", pkg)
	return nil
}

func findGocache() string {
	cmd := command("go env GOCACHE")
	cmd.Stdout = nil
	out, err := cmd.Output()
	x.Check(err)
	return "GOCACHE=" + string(out)
}

func runCommonTests(pkgCh chan string, closer *z.Closer) error {
	defer closer.Done()

	defaultCompose := path.Join(*baseDir, "dgraph/docker-compose.yml")
	id := atomic.AddInt32(&testId, 1)
	prefix := "test" + strconv.Itoa(int(id))

	stopCluster(defaultCompose, prefix)
	startCluster(defaultCompose, prefix)
	defer stopCluster(defaultCompose, prefix)

	// Wait for cluster to be healthy.
	getInstance(prefix, "alpha1").loginFatal()

	ctx := closer.Ctx()
	for pkg := range pkgCh {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := runTestsFor(ctx, pkg, prefix); err != nil {
			return err
		}
	}
	return nil
}

func findPackageFor(testName string) string {
	cmd := command(fmt.Sprintf("ack %s %s -l", testName, *baseDir))
	var b bytes.Buffer
	cmd.Stdout = &b
	if err := cmd.Run(); err != nil {
		fmt.Printf("Unable to find %s: %v\n", *runTest, err)
		return ""
	}
	scan := bufio.NewScanner(&b)
	for scan.Scan() {
		fname := scan.Text()
		if strings.HasSuffix(fname, "_test.go") {
			return path.Dir(fname)
		}
	}
	return ""
}

func main() {
	pflag.Parse()

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
	commonTestCh := make(chan string, N)
	errCh := make(chan error, 100)
	for i := 0; i < N; i++ {
		go func() {
			if err := runCommonTests(commonTestCh, closer); err != nil {
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
		defer close(commonTestCh)
		pattern := *baseDir + "/..."

		if len(*runTest) > 0 {
			pattern = findPackageFor(*runTest)
			fmt.Printf("Found package for %s: %s\n", *runTest, pattern)
		}
		pkgs, err := packages.Load(nil, pattern)
		x.Check(err)
		if len(pkgs) == 0 {
			fmt.Println("Couldn't find any packages. Exiting...")
			os.Exit(1)
		}

		valid := pkgs[:0]
		for _, pkg := range pkgs {
			if len(*runPkg) > 0 && !strings.HasSuffix(pkg.ID, *runPkg) {
				continue
			}
			valid = append(valid, pkg)
			fmt.Printf("Found valid package: %s\n", pkg.ID)
		}

		for _, pkg := range valid {
			dir := strings.Replace(pkg.ID, "github.com/dgraph-io/dgraph/", "", 1)
			fname := path.Join(*baseDir, dir, "docker-compose.yml")
			_, err := os.Stat(fname)
			if os.IsNotExist(err) {
				select {
				case commonTestCh <- pkg.ID:
				case <-closer.HasBeenClosed():
					return
				}
			} else {
				fmt.Printf("IGNORING tests for %s\n", pkg.ID)
				// Found it.
			}
		}
	}()

	closer.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			fmt.Printf("Got error: %v. Tests FAILED.\n", err)
			os.Exit(1)
		}
	}
	fmt.Println("Tests PASSED.")
}
