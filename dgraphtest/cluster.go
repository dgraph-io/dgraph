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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"go.uber.org/multierr"

	"github.com/dgraph-io/dgraph/testutil"
)

var (
	requestTimeout = 30 * time.Second
	stopTimeout    = time.Minute
)

type Cluster struct {
	conf ClusterConfig

	// resources
	dcli   *client.Client
	zeros  []*zero
	alphas []*alpha
}

func NewCluster(conf ClusterConfig) (*Cluster, error) {
	c := &Cluster{conf: conf}
	if err := c.init(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Cluster) init() error {
	var err error
	c.dcli, err = client.NewEnvClient()
	if err != nil {
		return errors.Wrap(err, "error setting up docker client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if _, err := c.dcli.Ping(ctx); err != nil {
		return errors.Wrap(err, "unable to talk to docker daemon")
	}

	err = c.createNetwork()
	if err != nil {
		panic(err)
	}
	for i := 1; i <= c.conf.numZeros; i++ {
		zo, err := c.createContainer(&zero{id: i})
		if err != nil {
			return err
		}
		c.zeros = append(c.zeros, &zero{containerId: zo})
	}
	for i := 1; i <= c.conf.numAlphas; i++ {
		aa, err := c.createContainer(&alpha{id: i})
		if err != nil {
			return err
		}
		c.alphas = append(c.alphas, &alpha{containerId: aa})
	}
	return nil
}

func (c *Cluster) Start() error {
	c.conf.w.Logf("Starting Cluster")
	for i := 0; i < c.conf.numZeros; i++ {
		if err := c.StartZero(i); err != nil {
			return err
		}
	}
	for i := 0; i < c.conf.numAlphas; i++ {
		if err := c.StartAlpha(i); err != nil {
			return err
		}
	}
	//check health
	c.healthCheck()
	return nil
}

func (c *Cluster) StartZero(id int) error {
	if id >= c.conf.numZeros {
		return fmt.Errorf("invalid id of zero: %v", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerStart(ctx, c.zeros[id].containerId, types.ContainerStartOptions{}); err != nil {
		return errors.Wrap(err, "error starting zero container")
	}
	return nil
}

func (c *Cluster) StartAlpha(id int) error {
	if id >= c.conf.numAlphas {
		return fmt.Errorf("invalid id of alpha: %v", id)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerStart(ctx, c.alphas[id].containerId, types.ContainerStartOptions{}); err != nil {
		return errors.Wrap(err, "error starting zero container")
	}
	return nil
}

func (c *Cluster) StopZero(id int) error {
	if id >= c.conf.numZeros {
		return fmt.Errorf("invalid id of zero: %v", id)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerStop(ctx, c.zeros[id].containerId, &stopTimeout); err != nil {
		return errors.Wrap(err, "error stopping zero container")
	}
	return nil
}

func (c *Cluster) StopAlpha(id int) error {
	if id >= c.conf.numAlphas {
		return fmt.Errorf("invalid id of alpha: %v", id)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerStop(ctx, c.alphas[id].containerId, &stopTimeout); err != nil {
		return errors.Wrap(err, "error stopping alpha container")
	}
	return nil
}

func (c *Cluster) Stop() error {
	c.conf.w.Logf("Stopping Cluster")
	for i := range c.alphas {
		if err := c.StopAlpha(i); err != nil {
			return err
		}
	}
	for i := range c.zeros {
		if err := c.StopZero(i); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) Cleanup() error {
	c.conf.w.Logf("Cleaning up cluster")

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	var merr error

	ro := types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
	for _, aa := range c.alphas {
		if err := c.dcli.ContainerRemove(ctx, aa.containerId, ro); err != nil {
			merr = multierr.Combine(merr, err)
		}
	}
	for _, zo := range c.zeros {
		if err := c.dcli.ContainerRemove(ctx, zo.containerId, ro); err != nil {
			merr = multierr.Append(merr, err)
		}
	}
	if err := c.dcli.NetworkRemove(ctx, c.conf.networkId); err != nil {
		merr = multierr.Append(merr, err)
	}
	return merr
}

func (c *Cluster) createContainer(dc dcontainer) (string, error) {
	name := dc.name(c.conf.prefix)
	ps := dc.ports()
	cmd := dc.cmd(c.conf)
	image := c.dgraphImage()
	wd := dc.workingDir()
	mts := dc.mounts(c.conf)
	cconf := &container.Config{Cmd: cmd, Image: image, WorkingDir: wd, ExposedPorts: ps}
	hconf := &container.HostConfig{Mounts: mts, PublishAllPorts: true}
	networkConfig := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{c.conf.prefix + "-net": {Aliases: []string{name}, NetworkID: c.conf.networkId}}}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	resp, err := c.dcli.ContainerCreate(ctx, cconf, hconf, networkConfig, name)
	if err != nil {
		return "", errors.Wrapf(err, "error creating container %v", name)
	}

	return resp.ID, nil
}

func (c *Cluster) UpgradeWithBinary() error {
	//stop all containers
	err := c.Stop()
	if err != nil {
		return errors.Wrapf(err, "error stopping containers  %v", err)
	}

	//setup function make binaries

	//start cluster
	err = c.Start()
	if err != nil {
		return errors.Wrapf(err, "error starting containers  %v", err)
	}

	return nil
}

func (c *Cluster) healthCheck() {
	c.conf.w.Logf("Checking health of containers")
	for i := 0; i < c.conf.numZeros; i++ {
		c.conf.w.Logf("Checking health of Zeros")
		containerInfo, err := c.dcli.ContainerInspect(context.Background(), c.zeros[i].containerId)
		if err != nil {
			panic(err)
		}
		publicPort := c.publicPort(containerInfo, "6080")
		err = c.bestHealthCheck(publicPort, c.zeros[i].containerName)
		if err != nil {
			panic(err)
		}

	}
	for i := 0; i < c.conf.numAlphas; i++ {
		c.conf.w.Logf("Checking health of Alphas")
		containerInfo, err := c.dcli.ContainerInspect(context.Background(), c.alphas[i].containerId)
		if err != nil {
			panic(err)
		}
		publicPort := c.publicPort(containerInfo, "8080")
		err = c.bestHealthCheck(publicPort, c.alphas[i].containerName)
		if err != nil {
			panic(err)
		}

	}
}

func (c *Cluster) publicPort(containerInfo types.ContainerJSON, privatePort string) string {
	var publicPort string
	for port, bindings := range containerInfo.NetworkSettings.Ports {
		if port.Port() == privatePort {
			publicPort = bindings[0].HostPort
		}
	}
	return publicPort
}

func (c *Cluster) bestHealthCheck(publicPort, containerName string) error {
	checkACL := func(body []byte) error {
		const acl string = "\"acl\""
		if bytes.Index(body, []byte(acl)) > 0 {
			return c.bestEffortTryLogin(containerName, publicPort)
		}
		return nil
	}

	for i := 0; i < 30; i++ {
		resp, err := http.Get("http://localhost:" + publicPort + "/health")
		var body []byte
		if resp != nil && resp.Body != nil {
			body, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
		}
		if err == nil && resp.StatusCode == http.StatusOK {
			return checkACL(body)
		}
		c.conf.w.Logf("Health for %s failed: %v. Response: %q. Retrying...\n", containerName, err, body)
		time.Sleep(time.Second)
	}
	return fmt.Errorf("did not pass health check on http://localhost:%v/health", publicPort)
}

func (c *Cluster) bestEffortTryLogin(containerName, publicPort string) error {
	for i := 0; i < 30; i++ {
		err := c.login(publicPort)
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
		c.conf.w.Logf("login failed for %s: %v. Retrying...\n", containerName, err)
		time.Sleep(time.Second)
	}
	c.conf.w.Logf("unable to login to %s\n", containerName)
	return fmt.Errorf("unable to login to %s", containerName)
}

func (c *Cluster) login(publicPort string) error {
	_, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: "http://localhost:" + publicPort + "/admin",
		UserID:   "groot",
		Passwd:   "password",
	})

	if err != nil {
		return fmt.Errorf("while connecting: %v", err)
	}
	return nil
}

func (c *Cluster) createNetwork() error {
	networkOptions := types.NetworkCreate{
		Driver: "bridge",
		IPAM: &network.IPAM{
			Driver: "default",
		},
	}
	// c.dcli.NetworkCreate(context.Background(), "my_network", networkOptions)
	network, err := c.dcli.NetworkCreate(context.Background(), c.conf.prefix+"-net", networkOptions)
	if err != nil {
		return err
	}
	c.conf.networkId = network.ID
	return nil
}

func (c *Cluster) GetPublicPortOfAlpha(index int, privatePort string) string {

	containerInfo, err := c.dcli.ContainerInspect(context.Background(), c.alphas[index].containerId)
	if err != nil {
		panic(err)
	}
	publicPort := c.publicPort(containerInfo, privatePort)
	return publicPort

}

func (c *Cluster) GetPublicPortOfZero(index int, privatePort string) string {

	containerInfo, err := c.dcli.ContainerInspect(context.Background(), c.zeros[index].containerId)
	if err != nil {
		panic(err)
	}
	publicPort := c.publicPort(containerInfo, privatePort)
	return publicPort

}
