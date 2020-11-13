package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
)

var (
	ctxb       = context.Background()
	isTeamcity bool

	baseDir = pflag.StringP("base", "", "../",
		"Base dir for Dgraph")
)

func commandWithContext(ctx context.Context, q string) *exec.Cmd {
	splits := strings.Split(q, " ")
	cmd := exec.CommandContext(ctx, splits[0], splits[1:]...)
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
	return fmt.Sprintf("%s %s", in.Prefix, in.Name)
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
	fmt.Printf("Got container: %+v\n", c)
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
func runTestsFor(pkg, prefix string) {
	q := fmt.Sprintf("go test -v %s", pkg)
	cmd := command(q)
	in := instance{
		Prefix: prefix,
		Name:   "alpha" + strconv.Itoa(1+rand.Intn(6)),
	}
	cmd.Env = append(cmd.Env, "TEST_PORT_ALPHA="+in.publicPort(9080))
	cmd.Env = append(cmd.Env, "TEST_PORT_ALPHA_HTTP="+in.publicPort(8080))

	in.Name = "zero" + strconv.Itoa(1)
	cmd.Env = append(cmd.Env, "TEST_PORT_ZERO="+in.publicPort(5080))
	cmd.Env = append(cmd.Env, "TEST_PORT_ZERO_HTTP="+in.publicPort(6080))

	fmt.Printf("Running: %s\n", cmd)
	if err := cmd.Run(); err != nil {
		log.Fatalf("While running command: %q Error: %v\n",
			q, err)
	}
	fmt.Printf("Ran tests for package: %s", pkg)
}

func findGocache() string {
	cmd := command("go env GOCACHE")
	cmd.Stdout = nil
	out, err := cmd.Output()
	x.Check(err)
	return "GOCACHE=" + string(out)
}
func main() {
	pflag.Parse()

	tmpDir, err := ioutil.TempDir("", "dgraph-test")
	x.Check(err)
	defer os.RemoveAll(tmpDir)

	if tc := os.Getenv("TEAMCITY_VERSION"); len(tc) > 0 {
		fmt.Printf("Found Teamcity: %s\n", tc)
		isTeamcity = true
	}

	defaultCompose := path.Join(*baseDir, "dgraph/docker-compose.yml")

	fmt.Printf("Using default Compose: %s\n", defaultCompose)

	stopCluster(defaultCompose, "test1")
	startCluster(defaultCompose, "test1")
	defer stopCluster(defaultCompose, "test1")

	fmt.Println("Cluster is up and running")
	getInstance("test1", "alpha2").loginFatal()

	runTestsFor(path.Join(*baseDir, "ee/acl"), "test1")
	fmt.Println("Log IN done")
}
