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

package testutil

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/x"
)

const (
	Start int = iota
	Stop
)

type ContainerInstance struct {
	Prefix string
	Name   string
}

func GetContainerInstance(prefix, name string) ContainerInstance {
	return ContainerInstance{Prefix: prefix, Name: name}
}

func (in ContainerInstance) String() string {
	return fmt.Sprintf("%s_%s_1", in.Prefix, in.Name)
}

func (in ContainerInstance) BestEffortWaitForHealthy(privatePort uint16) error {
	port := in.publicPort(privatePort)
	if len(port) == 0 {
		return nil
	}
	checkACL := func(body []byte) error {
		const acl string = "\"acl\""
		if bytes.Index(body, []byte(acl)) > 0 {
			return in.bestEffortTryLogin()
		}
		return nil
	}

	for i := 0; i < 30; i++ {
		resp, err := http.Get("http://localhost:" + port + "/health")
		var body []byte
		if resp != nil && resp.Body != nil {
			body, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
		}
		if err == nil && resp.StatusCode == http.StatusOK {
			return checkACL(body)
		}
		fmt.Printf("Health for %s failed: %v. Response: %q. Retrying...\n", in, err, body)
		time.Sleep(time.Second)
	}
	return fmt.Errorf("did not pass health check on %s", "http://localhost:"+port+"/health\n")
}

func (in ContainerInstance) publicPort(privatePort uint16) string {
	c := in.GetContainer()
	if c == nil {
		return ""
	}
	for _, p := range c.Ports {
		if p.PrivatePort == privatePort {
			return strconv.Itoa(int(p.PublicPort))
		}
	}
	return ""
}

func (in ContainerInstance) login() error {
	addr := in.publicPort(8080)
	if len(addr) == 0 {
		return fmt.Errorf("unable to find container: %s", in)
	}

	_, err := HttpLogin(&LoginParams{
		Endpoint: "http://localhost:" + addr + "/admin",
		UserID:   "groot",
		Passwd:   "password",
	})

	if err != nil {
		return fmt.Errorf("while connecting: %v", err)
	}
	fmt.Printf("Logged into %s\n", in)
	return nil
}

func (in ContainerInstance) bestEffortTryLogin() error {
	for i := 0; i < 30; i++ {
		err := in.login()
		if err == nil {
			return nil
		}
		if strings.Contains(err.Error(), "Invalid X-Dgraph-AuthToken") {
			// This is caused by Poor Man's auth. Return.
			return nil
		}
		if strings.Contains(err.Error(), "Client sent an HTTP request to an HTTPS server.") {
			// This is TLS enabled cluster. We won't be able to login.
			return nil
		}
		fmt.Printf("login failed for %s: %v. Retrying...\n", in, err)
		time.Sleep(time.Second)
	}
	fmt.Printf("unable to login to %s\n", in)
	return fmt.Errorf("unable to login to %s", in)
}

func (in ContainerInstance) GetContainer() *types.Container {
	containers := AllContainers(in.Prefix)

	q := fmt.Sprintf("/%s_%s_", in.Prefix, in.Name)
	for _, container := range containers {
		for _, name := range container.Names {
			if strings.HasPrefix(name, q) {
				return &container
			}
		}
	}
	return nil
}

func getContainer(name string) types.Container {
	cli, err := client.NewEnvClient()
	x.Check(err)

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		log.Fatalf("While listing container: %v\n", err)
	}

	q := fmt.Sprintf("/%s_%s_", DockerPrefix, name)
	for _, c := range containers {
		for _, n := range c.Names {
			if !strings.HasPrefix(n, q) {
				continue
			}
			return c
		}
	}
	return types.Container{}
}

func AllContainers(prefix string) []types.Container {
	cli, err := client.NewEnvClient()
	x.Check(err)

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
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

func ContainerAddr(name string, privatePort uint16) string {
	c := getContainer(name)
	for _, p := range c.Ports {
		if p.PrivatePort == privatePort {
			return "localhost:" + strconv.Itoa(int(p.PublicPort))
		}
	}
	return "localhost:" + strconv.Itoa(int(privatePort))
}

// DockerStart starts the specified services.
func DockerRun(instance string, op int) error {
	c := getContainer(instance)
	if c.ID == "" {
		glog.Fatalf("Unable to find container: %s\n", instance)
		return nil
	}

	cli, err := client.NewEnvClient()
	x.Check(err)

	switch op {
	case Start:
		if err := cli.ContainerStart(context.Background(), c.ID,
			types.ContainerStartOptions{}); err != nil {
			return err
		}
	case Stop:
		dur := 30 * time.Second
		return cli.ContainerStop(context.Background(), c.ID, &dur)
	default:
		x.Fatalf("Wrong Docker op: %v\n", op)
	}
	return nil
}

// MARKED FOR DEPRECATION: DockerCp copies from/to a container. Paths inside a container have the format
// container_name:path.
func DockerCp(srcPath, dstPath string) error {
	argv := []string{"docker", "cp", srcPath, dstPath}
	return Exec(argv...)
}

// DockerCpFromContainer copies from a container.
func DockerCpFromContainer(containerID, srcPath, dstPath string) error {
	cli, err := client.NewEnvClient()
	x.Check(err)

	tarStream, _, err := cli.CopyFromContainer(context.Background(), containerID, srcPath)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	tr := tar.NewReader(tarStream)
	_, err = tr.Next()
	x.Check(err)

	data, err := io.ReadAll(tr)
	x.Check(err)

	return os.WriteFile(dstPath, data, 0644)
}

// DockerExec executes a command inside the given container.
func DockerExec(instance string, cmd ...string) error {
	c := getContainer(instance)
	if c.ID == "" {
		glog.Fatalf("Unable to find container: %s\n", instance)
		return nil
	}
	argv := []string{"docker", "exec", "--user", "root", c.ID}
	argv = append(argv, cmd...)
	return Exec(argv...)
}

func DockerInspect(containerID string) (types.ContainerJSON, error) {
	cli, err := client.NewEnvClient()
	x.Check(err)
	return cli.ContainerInspect(context.Background(), containerID)
}

// checkHealthContainer checks health of container and determines wheather container is ready to accept request
func CheckHealthContainer(socketAddrHttp string) error {
	var err error
	var resp *http.Response
	url := "http://" + socketAddrHttp + "/health"
	for i := 0; i < 30; i++ {
		resp, err = http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			return nil
		}
		var body []byte
		if resp != nil && resp.Body != nil {
			body, err = io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			_ = resp.Body.Close()
		}
		fmt.Printf("health check for container failed: %v. Response: %q. Retrying...\n", err, body)
		time.Sleep(time.Second)

	}
	return err
}
