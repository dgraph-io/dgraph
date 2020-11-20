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

package testutil

import (
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

const (
	Start int = iota
	Stop
)

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

// DockerCp copies from/to a container. Paths inside a container have the format
// container_name:path.
func DockerCp(srcPath, dstPath string) error {
	argv := []string{"docker", "cp", srcPath, dstPath}
	return Exec(argv...)
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
