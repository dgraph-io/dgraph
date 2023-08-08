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
	"strconv"
	"time"

	"github.com/docker/docker/api/types/mount"
	docker "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
)

const (
	binaryNameFmt    = "dgraph_%v"
	zeroNameFmt      = "%v_zero%d"
	zeroAliasNameFmt = "zero%d"
	alphaNameFmt     = "%v_alpha%d"
	alphaLNameFmt    = "alpha%d"
	volNameFmt       = "%v_%v"

	zeroGrpcPort   = "5080"
	zeroHttpPort   = "6080"
	alphaInterPort = "7080"
	alphaHttpPort  = "8080"
	alphaGrpcPort  = "9080"

	alphaWorkingDir  = "/data/alpha"
	zeroWorkingDir   = "/data/zero"
	DefaultAlphaPDir = "/data/alpha/p"
	DefaultBackupDir = "/data/backups"
	DefaultExportDir = "/data/exports"

	aclSecretMountPath = "/dgraph-acl/hmac-secret"
	encKeyMountPath    = "/dgraph-enc/enc-key"

	DefaultUser     = "groot"
	DefaultPassword = "password"

	localVersion       = "local"
	waitDurBeforeRetry = time.Second
	requestTimeout     = 120 * time.Second
)

var (
	errNotImplemented = errors.New("NOT IMPLEMENTED")
)

func fileExists(filename string) (bool, error) {
	info, err := os.Stat(filename)
	if err != nil && !os.IsNotExist(err) {
		return false, errors.Wrap(err, "error while getting file info")
	}
	if err != nil && os.IsNotExist(err) {
		return false, nil
	}
	return !info.IsDir(), nil
}

type dnode interface {
	cname() string
	aname() string
	cid() string
	ports() nat.PortSet
	bindings(int) nat.PortMap
	cmd(*LocalCluster) []string
	workingDir() string
	mounts(*LocalCluster) ([]mount.Mount, error)
	healthURL(*LocalCluster) (string, error)
	assignURL(*LocalCluster) (string, error)
	alphaURL(*LocalCluster) (string, error)
	zeroURL(*LocalCluster) (string, error)
}

type zero struct {
	id            int    // 0, 1, 2
	containerID   string // container ID in docker world
	containerName string // something like test-1234_zero2
	aliasName     string // something like alpha0, zero1
}

func (z *zero) cname() string {
	return z.containerName
}

func (z *zero) aname() string {
	return z.aliasName
}

func (z *zero) cid() string {
	return z.containerID
}

func (z *zero) ports() nat.PortSet {
	return nat.PortSet{
		zeroGrpcPort: {},
		zeroHttpPort: {},
	}
}

func (z *zero) bindings(offset int) nat.PortMap {
	if offset < 0 {
		return nil
	}

	grpcPort, _ := strconv.Atoi(zeroGrpcPort)
	httpPort, _ := strconv.Atoi(zeroHttpPort)
	return nat.PortMap(map[nat.Port][]nat.PortBinding{
		zeroGrpcPort: {{HostPort: strconv.Itoa(grpcPort + offset + z.id)}},
		zeroHttpPort: {{HostPort: strconv.Itoa(httpPort + offset + z.id)}},
	})
}

func (z *zero) cmd(c *LocalCluster) []string {
	zcmd := []string{"/gobin/dgraph", "zero", fmt.Sprintf("--my=%s:%v", z.aname(), zeroGrpcPort), "--bindall",
		fmt.Sprintf(`--replicas=%v`, c.conf.replicas), fmt.Sprintf(`--raft=idx=%v`, z.id+1), "--logtostderr",
		fmt.Sprintf("-v=%d", c.conf.verbosity),
		fmt.Sprintf(`--limit=refill-interval=%v;uid-lease=%v`, c.conf.refillInterval, c.conf.uidLease)}

	if z.id > 0 {
		zcmd = append(zcmd, "--peer="+c.zeros[0].aname()+":"+zeroGrpcPort)
	}

	return zcmd
}

func (z *zero) workingDir() string {
	return zeroWorkingDir
}

func (z *zero) mounts(c *LocalCluster) ([]mount.Mount, error) {
	var mounts []mount.Mount
	binMount, err := mountBinary(c)
	if err != nil {
		return nil, err
	}
	mounts = append(mounts, binMount)
	return mounts, nil
}

func (z *zero) healthURL(c *LocalCluster) (string, error) {
	publicPort, err := publicPort(c.dcli, z, zeroHttpPort)
	if err != nil {
		return "", err
	}
	return "http://localhost:" + publicPort + "/health", nil
}

func (z *zero) assignURL(c *LocalCluster) (string, error) {
	publicPort, err := publicPort(c.dcli, z, zeroHttpPort)
	if err != nil {
		return "", err
	}
	return "http://localhost:" + publicPort + "/assign", nil
}

func (z *zero) alphaURL(c *LocalCluster) (string, error) {
	return "", errNotImplemented
}

func (z *zero) zeroURL(c *LocalCluster) (string, error) {
	publicPort, err := publicPort(c.dcli, z, zeroGrpcPort)
	if err != nil {
		return "", err
	}
	return "localhost:" + publicPort + "", nil
}

type alpha struct {
	id            int
	containerID   string
	containerName string
	aliasName     string
}

func (a *alpha) cname() string {
	return a.containerName
}

func (a *alpha) cid() string {
	return a.containerID
}

func (a *alpha) aname() string {
	return a.aliasName
}

func (a *alpha) ports() nat.PortSet {
	return nat.PortSet{
		alphaGrpcPort: {},
		alphaHttpPort: {},
	}
}

func (a *alpha) bindings(offset int) nat.PortMap {
	if offset < 0 {
		return nil
	}

	grpcPort, _ := strconv.Atoi(alphaGrpcPort)
	httpPort, _ := strconv.Atoi(alphaHttpPort)
	return nat.PortMap(map[nat.Port][]nat.PortBinding{
		alphaGrpcPort: {{HostPort: strconv.Itoa(grpcPort + offset + a.id)}},
		alphaHttpPort: {{HostPort: strconv.Itoa(httpPort + offset + a.id)}},
	})
}

func (a *alpha) cmd(c *LocalCluster) []string {
	acmd := []string{"/gobin/dgraph", "alpha", fmt.Sprintf("--my=%s:%v", a.aname(), alphaInterPort),
		"--bindall", "--logtostderr", fmt.Sprintf("-v=%d", c.conf.verbosity),
		`--security=whitelist=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16`}

	if c.conf.acl {
		acmd = append(acmd, fmt.Sprintf(`--acl=secret-file=%s;access-ttl=%s`, aclSecretMountPath, c.conf.aclTTL))
	}
	if c.conf.encryption {
		acmd = append(acmd, fmt.Sprintf(`--encryption=key-file=%v`, encKeyMountPath))
	}

	zeroAddrsArg, delimiter := "--zero=", ""
	for _, zo := range c.zeros {
		zeroAddrsArg += fmt.Sprintf("%s%v:%v", delimiter, zo.aname(), zeroGrpcPort)
		delimiter = ","
	}
	acmd = append(acmd, zeroAddrsArg)

	if c.conf.lambdaURL != "" {
		acmd = append(acmd, fmt.Sprintf(`--graphql=lambda-url=%s`, c.conf.lambdaURL))
	}

	return acmd
}

func (a *alpha) workingDir() string {
	return alphaWorkingDir
}

func (a *alpha) mounts(c *LocalCluster) ([]mount.Mount, error) {
	var mounts []mount.Mount
	binMount, err := mountBinary(c)
	if err != nil {
		return nil, err
	}
	mounts = append(mounts, binMount)

	if c.conf.acl {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   aclSecretPath,
			Target:   aclSecretMountPath,
			ReadOnly: true,
		})
	}

	if c.conf.encryption {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   encKeyPath,
			Target:   encKeyMountPath,
			ReadOnly: true,
		})
	}

	if c.conf.bulkOutDir != "" {
		if c.conf.numAlphas == 1 || (c.conf.numAlphas > 1 && a.id == 0) {
			pDir := filepath.Join(c.conf.bulkOutDir, strconv.Itoa(a.id/c.conf.replicas), "p")
			if err := os.MkdirAll(pDir, os.ModePerm); err != nil {
				return nil, errors.Wrap(err, "erorr creating bulk dir")
			}
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   pDir,
				Target:   DefaultAlphaPDir,
				ReadOnly: false,
			})
		}

	}

	for dir, vol := range c.conf.volumes {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeVolume,
			Source:   vol,
			Target:   dir,
			ReadOnly: false,
		})
	}
	return mounts, nil
}

func (a *alpha) healthURL(c *LocalCluster) (string, error) {
	publicPort, err := publicPort(c.dcli, a, alphaHttpPort)
	if err != nil {
		return "", err
	}
	return "http://localhost:" + publicPort + "/health", nil
}

func (a *alpha) assignURL(c *LocalCluster) (string, error) {
	return "", errors.New("no assign URL for alpha")
}

func (a *alpha) alphaURL(c *LocalCluster) (string, error) {
	publicPort, err := publicPort(c.dcli, a, alphaGrpcPort)
	if err != nil {
		return "", err
	}
	return "localhost:" + publicPort + "", nil
}

func (a *alpha) zeroURL(c *LocalCluster) (string, error) {
	return "", errNotImplemented
}

func publicPort(dcli *docker.Client, dc dnode, privatePort string) (string, error) {
	// TODO(aman): we should cache the port information
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	info, err := dcli.ContainerInspect(ctx, dc.cid())
	if err != nil {
		return "", errors.Wrap(err, "error inspecting container")
	}

	for port, bindings := range info.NetworkSettings.Ports {
		if len(bindings) == 0 {
			continue
		}
		if port.Port() == privatePort {
			return bindings[0].HostPort, nil
		}
	}

	return "", fmt.Errorf("no mapping found for private port [%v] for container [%v]", privatePort, dc.cname())
}

func mountBinary(c *LocalCluster) (mount.Mount, error) {
	if err := c.setupBinary(); err != nil {
		return mount.Mount{}, err
	}
	return mount.Mount{
		Type:     mount.TypeBind,
		Source:   c.tempBinDir,
		Target:   "/gobin",
		ReadOnly: true,
	}, nil
}
