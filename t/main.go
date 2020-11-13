package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
)

var (
	ctxb = context.Background()

	baseDir = pflag.StringP("base", "", "../",
		"Base dir for Dgraph")
)

func commandWithContext(ctx context.Context, q string) *exec.Cmd {
	splits := strings.Split(q, " ")
	cmd := exec.CommandContext(ctx, splits[0], splits[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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
func getContainer(prefix, name string) types.Container {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	containers, err := cli.ContainerList(ctxb, types.ContainerListOptions{})
	if err != nil {
		log.Fatalf("While listing container: %v\n", err)
	}

	q := fmt.Sprintf("/%s_%s_", prefix, name)
	for _, container := range containers {
		for _, name := range container.Names {
			if strings.HasPrefix(name, q) {
				return container
			}
		}
	}
	return types.Container{}
}
func getIPAddr(prefix, name string) string {
	c := getContainer(prefix, name)
	// fmt.Printf("Got container: %+v\n", c)
	for _, p := range c.Ports {
		if p.PrivatePort == 9080 {
			return strconv.Itoa(int(p.PublicPort))
		}
	}
	return ""
}
func login(prefix, name string) error {
	addr := getIPAddr(prefix, name)
	if len(addr) == 0 {
		return fmt.Errorf("unable to find container: %s %s", prefix, name)
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
	fmt.Printf("Logged into %s %s\n", prefix, name)
	return nil
}
func loginFatal(prefix, name string) error {
	for i := 0; i < 30; i++ {
		err := login(prefix, name)
		if err == nil {
			return nil
		}
		fmt.Printf("Login failed: %v. Retrying...\n", err)
		time.Sleep(time.Second)
	}
	glog.Fatalf("Unable to login to %s %s\n", prefix, name)
	return nil
}

func main() {
	pflag.Parse()

	defaultCompose := path.Join(*baseDir, "dgraph/docker-compose.yml")

	fmt.Printf("Using default Compose: %s\n", defaultCompose)

	startCluster(defaultCompose, "test1")
	defer stopCluster(defaultCompose, "test1")

	startCluster(defaultCompose, "test2")
	defer stopCluster(defaultCompose, "test2")

	fmt.Println("Cluster is up and running")
	loginFatal("test1", "alpha2")
	loginFatal("test2", "alpha3")
	fmt.Println("Log IN done")
}
