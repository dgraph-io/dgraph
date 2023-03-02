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
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
)

const (
	zeroNameFmt  = "%v-1_zero%d_1"
	alphaNameFmt = "%v-1_alpha%d_1"
	//remove
	alphaWorkingDir = "/data/alpha"
	zeroWorkingDir  = "/data/zero"
	zeroGrpcPort    = "5080"
	zeroHttpPort    = "6080"
	alphaGrpcPort   = "9080"
	alphaHttpPort   = "8080"
)

type dcontainer interface {
	name(string) string
	ports() nat.PortSet
	cmd(ClusterConfig) []string
	workingDir() string
	mounts(ClusterConfig) []mount.Mount
}

type zero struct {
	id            int
	containerId   string
	containerName string
}

func (z *zero) name(prefix string) string {
	z.containerName = fmt.Sprintf(zeroNameFmt, prefix, z.id)
	return z.containerName
}

func (z *zero) ports() nat.PortSet {
	return nat.PortSet{
		zeroGrpcPort: {},
		zeroHttpPort: {},
	}
}

func (z *zero) cmd(conf ClusterConfig) []string {
	zcmd := []string{"/gobin/dgraph", "zero", fmt.Sprintf("--my=%s:5080", z.containerName), "--bindall", fmt.Sprintf(`--replicas=%v`, conf.replicas),
		fmt.Sprintf(`--raft=idx=%v`, z.id), "--logtostderr", fmt.Sprintf("-v=%d", conf.verbosity)}

	if z.id > 1 {
		zcmd = append(zcmd, "--peer="+z.containerName+":5080")
	}

	return zcmd
}

func (z *zero) workingDir() string {
	return zeroWorkingDir
}

func (z *zero) mounts(conf ClusterConfig) []mount.Mount {
	mounts := []mount.Mount{}
	if conf.isUpgrade {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   "./upgrade/old_version",
			Target:   "/gobin",
			ReadOnly: true,
		})
	} else {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   os.Getenv("GOPATH") + "/bin",
			Target:   "/gobin",
			ReadOnly: true,
		})
	}
	return mounts
}

type alpha struct {
	id            int
	containerId   string
	containerName string
}

func (a *alpha) name(prefix string) string {
	a.containerName = fmt.Sprintf(alphaNameFmt, prefix, a.id)
	return a.containerName
}

func (a *alpha) ports() nat.PortSet {
	return nat.PortSet{
		alphaGrpcPort: {},
		alphaHttpPort: {},
	}
}

func (a *alpha) cmd(conf ClusterConfig) []string {
	acmd := []string{"/gobin/dgraph", "alpha", fmt.Sprintf("--my=%s:7080", a.containerName), "--bindall", "--logtostderr", fmt.Sprintf("-v=%d", conf.verbosity), `--security=whitelist=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16`}

	zeroAddrs, delimiter := "--zero=", ""
	if conf.acl {
		acmd = append(acmd, `--acl=secret-file=/dgraph-acl/hmac-secret ; access-ttl=20s`)
	}
	if conf.encryption {
		acmd = append(acmd, `--encryption=key-file=/dgraph-enc/enc-key`)
	}
	for i := 1; i <= conf.numZeros; i++ {
		zeroAddrs += fmt.Sprintf("%s%v-1_zero%d_1:5080", delimiter, conf.prefix, i)
		delimiter = ","
	}
	acmd = append(acmd, zeroAddrs)

	return acmd
}

func (a *alpha) workingDir() string {
	return alphaWorkingDir
}

func (a *alpha) mounts(conf ClusterConfig) []mount.Mount {
	mounts := []mount.Mount{}
	if conf.isUpgrade {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   "./upgrade/old_version",
			Target:   "/gobin",
			ReadOnly: true,
		})
	} else {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   os.Getenv("GOPATH") + "/bin",
			Target:   "/gobin",
			ReadOnly: true,
		})
	}

	if conf.acl {
		absPath, err := filepath.Abs("data/hmac-secret")
		if err != nil {
			fmt.Print("error while getting abs path")
		}
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   absPath,
			Target:   "/dgraph-acl/hmac-secret",
			ReadOnly: true,
		})
	}
	if conf.encryption {
		absPath, err := filepath.Abs("../dgraphtest/data/enc-key")
		if err != nil {
			fmt.Print("error while getting abs path")
		}
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   absPath,
			Target:   "/dgraph-enc/enc-key",
			ReadOnly: true,
		})
	}
	return mounts
}
