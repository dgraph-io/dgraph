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

package dgraphtest

import (
	"fmt"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
)

const (
	zeroNameFmt  = "%v-zero%02d"
	alphaNameFmt = "%v-alpha%02d"
)

type dcontainer interface {
	name(string) string
	ports() nat.PortSet
	cmd(ClusterConfig) []string
	workingDir() string
	mounts() []mount.Mount
}

type zero struct {
	id int
}

func (z zero) name(prefix string) string {
	return fmt.Sprintf(zeroNameFmt, prefix, z.id)
}

func (z zero) ports() nat.PortSet {
	return nat.PortSet{
		"5080": struct{}{},
		"6080": struct{}{},
	}
}

func (z zero) cmd(conf ClusterConfig) []string {
	zcmd := []string{"/gobin/dgraph", "zero", fmt.Sprintf("--my=zero%d:5080", z.id), "--bindall",
		fmt.Sprintf(`--raft="idx=%v"`, z.id), "--logtostderr", fmt.Sprintf("-v=%d", conf.verbosity)}

	if conf.numZeros > 1 {
		zcmd = append(zcmd, "--peer=zero1:5080")
	}

	return zcmd
}

func (z zero) workingDir() string {
	return "/data/zero"
}

func (z zero) mounts() []mount.Mount {
	return []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   "/home/aman/gocode/bin",
			Target:   "/gobin",
			ReadOnly: true,
		},
	}
}

type alpha struct {
	id int
}

func (a alpha) name(prefix string) string {
	return fmt.Sprintf(alphaNameFmt, prefix, a.id)
}

func (a alpha) ports() nat.PortSet {
	return nat.PortSet{
		"8080": struct{}{},
		"9080": struct{}{},
	}
}

func (a alpha) cmd(conf ClusterConfig) []string {
	zcmd := []string{"/gobin/dgraph", "alpha", fmt.Sprintf("--my=alpha%d:7080", a.id), "--bindall",
		fmt.Sprintf("--replicas:%v", conf.replicas), "--logtostderr", fmt.Sprintf("-v=%d", conf.verbosity)}

	zeroAddrs, delimiter := "--zero=", ""
	for i := 0; i < conf.numZeros; i++ {
		zeroAddrs += fmt.Sprintf("%szero%d:5080", delimiter, i)
		delimiter = ","
	}

	return zcmd
}

func (a alpha) workingDir() string {
	return "/data/alpha"
}

func (a alpha) mounts() []mount.Mount {
	return []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   "/home/aman/gocode/bin",
			Target:   "/gobin",
			ReadOnly: true,
		},
	}
}
