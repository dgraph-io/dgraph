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
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"go.uber.org/multierr"
)

var (
	requestTimeout = 30 * time.Second
	stopTimeout    = time.Minute
)

type Cluster struct {
	conf ClusterConfig

	// resources
	dcli   *client.Client
	zeros  []string
	alphas []string
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

	for i := len(c.zeros); i < c.conf.numZeros; i++ {
		zo, err := c.createContainer(zero{id: i})
		if err != nil {
			return err
		}
		c.zeros = append(c.zeros, zo)
	}
	for i := len(c.alphas); i < c.conf.numAlphas; i++ {
		aa, err := c.createContainer(alpha{id: i})
		if err != nil {
			return err
		}
		c.alphas = append(c.alphas, aa)
	}
	return nil
}

func (c *Cluster) Start() error {
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
	return nil
}

func (c *Cluster) StartZero(id int) error {
	if id >= c.conf.numZeros {
		return fmt.Errorf("invalid id of zero: %v", id)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerStart(ctx, c.zeros[id], types.ContainerStartOptions{}); err != nil {
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
	if err := c.dcli.ContainerStart(ctx, c.alphas[id], types.ContainerStartOptions{}); err != nil {
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
	if err := c.dcli.ContainerStop(ctx, c.zeros[id], &stopTimeout); err != nil {
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
	if err := c.dcli.ContainerStop(ctx, c.alphas[id], &stopTimeout); err != nil {
		return errors.Wrap(err, "error stopping alpha container")
	}
	return nil
}

func (c *Cluster) Stop() error {
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
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	var merr error
	ro := types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
	for _, aa := range c.alphas {
		if err := c.dcli.ContainerRemove(ctx, aa, ro); err != nil {
			merr = multierr.Combine(merr, err)
		}
	}
	for _, zo := range c.zeros {
		if err := c.dcli.ContainerRemove(ctx, zo, ro); err != nil {
			merr = multierr.Append(merr, err)
		}
	}
	return merr
}

func (c *Cluster) createContainer(dc dcontainer) (string, error) {
	name := dc.name(c.conf.prefix)
	ps := dc.ports()
	cmd := dc.cmd(c.conf)
	image := c.dgraphImage()
	wd := dc.workingDir()
	mts := dc.mounts()

	cconf := &container.Config{ExposedPorts: ps, Cmd: cmd, Image: image, WorkingDir: wd}
	hconf := &container.HostConfig{Mounts: mts}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	resp, err := c.dcli.ContainerCreate(ctx, cconf, hconf, nil, name)
	if err != nil {
		return "", errors.Wrapf(err, "error creating container %v", name)
	}

	return resp.ID, nil
}
