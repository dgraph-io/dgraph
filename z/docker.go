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

package z

import (
	"time"
)

// DockerStart starts the specified services.
func DockerStart(services ...string) error {
	argv := []string{"docker", "start"}
	argv = append(argv, services...)
	err := Exec(argv...)
	time.Sleep(time.Second)
	return err
}

// DockerStop stops the specified services.
func DockerStop(services ...string) error {
	argv := []string{"docker", "stop"}
	argv = append(argv, services...)
	return Exec(argv...)
}

// DockerPause pauses the specified services.
func DockerPause(services ...string) error {
	argv := []string{"docker", "pause"}
	argv = append(argv, services...)
	return Exec(argv...)
}

// DockerUnpause unpauses the specified services.
func DockerUnpause(services ...string) error {
	argv := []string{"docker", "unpause"}
	argv = append(argv, services...)
	err := Exec(argv...)
	time.Sleep(time.Second)
	return err
}

// DockerCp copies from/to a container. Paths inside a container have the format
// container_name:path.
func DockerCp(srcPath, dstPath string) error {
	argv := []string{"docker", "cp"}
	argv = append(argv, srcPath)
	argv = append(argv, dstPath)
	return Exec(argv...)
}

// DockerExec executes a command inside the given container.
func DockerExec(container string, cmd ...string) error {
	argv := []string{"docker", "exec", container}
	argv = append(argv, cmd...)
	return Exec(argv...)
}
