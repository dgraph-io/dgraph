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
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/mount"
	docker "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
)

const (
	zeroNameFmt   = "%v_zero%d"
	zeroLNameFmt  = "zero%d"
	alphaNameFmt  = "%v_alpha%d"
	alphaLNameFmt = "alpha%d"

	zeroGrpcPort   = "5080"
	zeroHttpPort   = "6080"
	alphaInterPort = "7080"
	alphaHttpPort  = "8080"
	alphaGrpcPort  = "9080"

	alphaWorkingDir = "/data/alpha"
	zeroWorkingDir  = "/data/zero"

	aclSecretPath      = "data/hmac-secret"
	aclSecretMountPath = "/dgraph-acl/hmac-secret"
	encKeyPath         = "data/enc-key"
	encKeyMountPath    = "/dgraph-enc/enc-key"
)

type dnode interface {
	cname() string
	lname() string
	cid() string
	ports() nat.PortSet
	cmd(*Cluster) []string
	workingDir() string
	mounts(ClusterConfig) ([]mount.Mount, error)
	healthURL(c *Cluster) (string, error)
}

type zero struct {
	id     int    // 0, 1, 2
	coid   string // container ID in docker world
	coname string // something like test-1234_zero2
	loname string // something like alpha0, zero1
}

func (z *zero) cname() string {
	return z.coname
}

func (z *zero) lname() string {
	return z.loname
}

func (z *zero) cid() string {
	return z.coid
}

func (z *zero) ports() nat.PortSet {
	return nat.PortSet{
		zeroGrpcPort: {},
		zeroHttpPort: {},
	}
}

func (z *zero) cmd(c *Cluster) []string {
	zcmd := []string{"/gobin/dgraph", "zero", fmt.Sprintf("--my=%s:%v", z.lname(), zeroGrpcPort), "--bindall",
		fmt.Sprintf(`--replicas=%v`, c.conf.replicas), fmt.Sprintf(`--raft=idx=%v`, z.id+1), "--logtostderr",
		fmt.Sprintf("-v=%d", c.conf.verbosity)}
	if z.id > 0 {
		zcmd = append(zcmd, "--peer="+c.zeros[0].lname()+":"+zeroGrpcPort)
	}

	return zcmd
}

func (z *zero) workingDir() string {
	return zeroWorkingDir
}

func (z *zero) mounts(conf ClusterConfig) ([]mount.Mount, error) {
	return []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   os.Getenv("GOPATH") + "/bin",
			Target:   "/gobin",
			ReadOnly: true,
		},
	}, nil
}

func (z *zero) healthURL(c *Cluster) (string, error) {
	publicPort, err := publicPort(c.dcli, z, zeroHttpPort)
	if err != nil {
		return "", err
	}
	return "http://localhost:" + publicPort + "/health", nil
}

type alpha struct {
	id     int
	coid   string
	coname string
	loname string
}

func (a *alpha) cname() string {
	return a.coname
}

func (a *alpha) cid() string {
	return a.coid
}

func (a *alpha) lname() string {
	return a.loname
}

func (a *alpha) ports() nat.PortSet {
	return nat.PortSet{
		alphaGrpcPort: {},
		alphaHttpPort: {},
	}
}

func (a *alpha) cmd(c *Cluster) []string {
	acmd := []string{"/gobin/dgraph", "alpha", fmt.Sprintf("--my=%s:%v", a.lname(), alphaInterPort),
		"--bindall", "--logtostderr", fmt.Sprintf("-v=%d", c.conf.verbosity),
		`--security=whitelist=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16`}

	if c.conf.acl {
		acmd = append(acmd, fmt.Sprintf(`--acl=secret-file=%v; access-ttl=%v`, aclSecretMountPath, c.conf.accessTTL))
	}
	if c.conf.encryption {
		acmd = append(acmd, fmt.Sprintf(`--encryption=key-file=%v`, encKeyMountPath))
	}

	zeroAddrsArg, delimiter := "--zero=", ""
	for _, zo := range c.zeros {
		zeroAddrsArg += fmt.Sprintf("%s%v:%v", delimiter, zo.lname(), zeroGrpcPort)
		delimiter = ","
	}
	acmd = append(acmd, zeroAddrsArg)

	return acmd
}

func (a *alpha) workingDir() string {
	return alphaWorkingDir
}

func (a *alpha) mounts(conf ClusterConfig) ([]mount.Mount, error) {
	mounts := []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   os.Getenv("GOPATH") + "/bin",
			Target:   "/gobin",
			ReadOnly: true,
		},
	}

	if conf.acl {
		absPath, err := filepath.Abs(aclSecretPath)
		if err != nil {
			return nil, errors.Wrap(err, "error finding absolute path for acl secret path")
		}
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   absPath,
			Target:   aclSecretMountPath,
			ReadOnly: true,
		})
	}

	if conf.encryption {
		absPath, err := filepath.Abs(encKeyPath)
		if err != nil {
			return nil, errors.Wrap(err, "error finding absolute path for enc key path")
		}
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   absPath,
			Target:   encKeyMountPath,
			ReadOnly: true,
		})
	}

	return mounts, nil
}

func (a *alpha) healthURL(c *Cluster) (string, error) {
	publicPort, err := publicPort(c.dcli, a, alphaHttpPort)
	if err != nil {
		return "", err
	}
	return "http://localhost:" + publicPort + "/health", nil
}

func publicPort(dcli *docker.Client, dc dnode, privatePort string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	info, err := dcli.ContainerInspect(ctx, dc.cid())
	if err != nil {
		return "", errors.Wrap(err, "error inspecting container")
	}

	for port, bindings := range info.NetworkSettings.Ports {
		if port.Port() == privatePort {
			return bindings[0].HostPort, nil
		}
	}

	return "", fmt.Errorf("no mapping found for private port [%v] for container [%v]", privatePort, dc.cname())
}
