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
	"io"
	"net/http"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
	"github.com/pkg/errors"
)

var (
	requestTimeout = 30 * time.Second
	stopTimeout    = time.Minute
)

// cluster's network struct
type cnet struct {
	id   string
	name string
}

type Cluster struct {
	conf ClusterConfig

	// resources
	dcli   *docker.Client
	net    cnet
	zeros  []dnode
	alphas []dnode
}

func NewCluster(conf ClusterConfig) (Cluster, error) {
	c := Cluster{conf: conf}
	if err := c.init(); err != nil {
		c.Cleanup()
		return Cluster{}, err
	}

	return c, nil
}

func (c *Cluster) log(format string, args ...any) {
	if c == nil || c.conf.logr == nil {
		return
	}
	c.conf.logr.Logf(format, args...)
}

func (c *Cluster) init() error {
	var err error
	c.dcli, err = docker.NewEnvClient()
	if err != nil {
		return errors.Wrap(err, "error setting up docker client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if _, err := c.dcli.Ping(ctx); err != nil {
		return errors.Wrap(err, "unable to talk to docker daemon")
	}

	if err := c.createNetwork(); err != nil {
		return errors.Wrap(err, "error creating network")
	}

	for i := 0; i < c.conf.numZeros; i++ {
		zo := &zero{id: i}
		zo.containerName = fmt.Sprintf(zeroNameFmt, c.conf.prefix, zo.id)
		zo.aliasName = fmt.Sprintf(zeroAliasNameFmt, zo.id)
		cid, err := c.createContainer(zo)
		if err != nil {
			return err
		}
		zo.containerID = cid
		c.zeros = append(c.zeros, zo)
	}

	for i := 0; i < c.conf.numAlphas; i++ {
		aa := &alpha{id: i}
		aa.containerName = fmt.Sprintf(alphaNameFmt, c.conf.prefix, aa.id)
		aa.aliasName = fmt.Sprintf(alphaLNameFmt, aa.id)
		cid, err := c.createContainer(aa)
		if err != nil {
			return err
		}
		aa.containerID = cid
		c.alphas = append(c.alphas, aa)
	}

	return nil
}

func (c *Cluster) createNetwork() error {
	c.net.name = c.conf.prefix + "-net"
	opts := types.NetworkCreate{
		Driver: "bridge",
		IPAM:   &network.IPAM{Driver: "default"},
	}

	network, err := c.dcli.NetworkCreate(context.Background(), c.net.name, opts)
	if err != nil {
		return errors.Wrap(err, "error creating network")
	}
	c.net.id = network.ID

	return nil
}

func (c *Cluster) Start() error {
	c.log("starting cluster with prefix [%v]", c.conf.prefix)
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

	if err := c.healthCheck(); err != nil {
		return err
	}
	return nil
}

func (c *Cluster) StartZero(id int) error {
	if id >= c.conf.numZeros {
		return fmt.Errorf("invalid id of zero: %v", id)
	}
	return c.startContainer(c.zeros[id])
}

func (c *Cluster) StartAlpha(id int) error {
	if id >= c.conf.numAlphas {
		return fmt.Errorf("invalid id of alpha: %v", id)
	}
	return c.startContainer(c.alphas[id])
}

func (c *Cluster) startContainer(dc dnode) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerStart(ctx, dc.cid(), types.ContainerStartOptions{}); err != nil {
		return errors.Wrapf(err, "error starting container [%v]", dc.cname())
	}
	return nil
}

func (c *Cluster) Stop() error {
	c.log("stopping cluster with prefix [%v]", c.conf.prefix)
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

func (c *Cluster) StopZero(id int) error {
	if id >= c.conf.numZeros {
		return fmt.Errorf("invalid id of zero: %v", id)
	}
	return c.stopContainer(c.zeros[id])
}

func (c *Cluster) StopAlpha(id int) error {
	if id >= c.conf.numAlphas {
		return fmt.Errorf("invalid id of alpha: %v", id)
	}
	return c.stopContainer(c.alphas[id])
}

func (c *Cluster) stopContainer(dc dnode) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerStop(ctx, dc.cid(), &stopTimeout); err != nil {
		return errors.Wrapf(err, "error stopping container [%v]", dc.cname())
	}
	return nil
}

func (c *Cluster) Cleanup() {
	c.log("cleaning up cluster with prefix [%v]", c.conf.prefix)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ro := types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
	for _, aa := range c.alphas {
		if err := c.dcli.ContainerRemove(ctx, aa.cid(), ro); err != nil {
			c.log("error removing alpha [%v]: %v", aa.cname(), err)
		}
	}
	for _, zo := range c.zeros {
		if err := c.dcli.ContainerRemove(ctx, zo.cid(), ro); err != nil {
			c.log("error removing zero [%v]: %v", zo.cname(), err)
		}
	}
	if err := c.dcli.NetworkRemove(ctx, c.net.id); err != nil {
		c.log("error removing network [%v]: %v", c.net.name, err)
	}
}

func (c *Cluster) createContainer(dc dnode) (string, error) {
	cmd := dc.cmd(c)
	image := c.dgraphImage()
	mts, err := dc.mounts(c.conf)
	if err != nil {
		return "", err
	}

	cconf := &container.Config{Cmd: cmd, Image: image, WorkingDir: dc.workingDir(), ExposedPorts: dc.ports()}
	hconf := &container.HostConfig{Mounts: mts, PublishAllPorts: true}
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			c.net.name: {
				Aliases:   []string{dc.cname(), dc.aname()},
				NetworkID: c.net.id,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	resp, err := c.dcli.ContainerCreate(ctx, cconf, hconf, networkConfig, dc.cname())
	if err != nil {
		return "", errors.Wrapf(err, "error creating container %v", dc.cname())
	}

	return resp.ID, nil
}

func (c *Cluster) healthCheck() error {
	c.log("checking health of containers")
	for i := 0; i < c.conf.numZeros; i++ {
		url, err := c.zeros[i].healthURL(c)
		if err != nil {
			return errors.Wrap(err, "error getting health URL")
		}
		if err := c.containerHealthCheck(url); err != nil {
			return err
		}
	}
	for i := 0; i < c.conf.numAlphas; i++ {
		url, err := c.alphas[i].healthURL(c)
		if err != nil {
			return errors.Wrap(err, "error getting health URL")
		}
		if err := c.containerHealthCheck(url); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) containerHealthCheck(url string) error {
	for i := 0; i < 60; i++ {
		resp, err := http.Get(url)
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			return nil
		}

		var body []byte
		if resp != nil && resp.Body != nil {
			body, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
		}

		c.log("health for [%v] failed, err: [%v], response: [%v]", url, err, body)
		time.Sleep(time.Second)
	}

	return fmt.Errorf("failed health check on [%v]", url)
}
