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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
)

var (
	requestTimeout = 60 * time.Second
	stopTimeout    = time.Minute
)

// cluster's network struct
type cnet struct {
	id   string
	name string
}

type LocalCluster struct {
	conf       ClusterConfig
	tempBinDir string
	// resources
	conns  []*grpc.ClientConn
	client *dgo.Dgraph
	dcli   *docker.Client
	net    cnet
	zeros  []dnode
	alphas []dnode
}

func NewLocalCluster(conf ClusterConfig) (*LocalCluster, error) {
	c := &LocalCluster{conf: conf}
	if err := c.init(); err != nil {
		glog.Infof(err.Error())
		c.Cleanup()
		return nil, err
	}

	return c, nil
}

func (c *LocalCluster) init() error {
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
	c.tempBinDir, err = os.MkdirTemp("", c.conf.prefix)
	if err != nil {
		return errors.Wrap(err, "error while creating temp dir")
	}
	if err := os.Mkdir(binDir, os.ModePerm); err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "error while making binDir")
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

func (c *LocalCluster) createNetwork() error {
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

func (c *LocalCluster) Start() error {
	glog.Infof("starting cluster with prefix [%v]", c.conf.prefix)
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
	return c.healthCheck()
}

func (c *LocalCluster) StartZero(id int) error {
	if id >= c.conf.numZeros {
		return fmt.Errorf("invalid id of zero: %v", id)
	}
	return c.startContainer(c.zeros[id])
}

func (c *LocalCluster) StartAlpha(id int) error {
	if id >= c.conf.numAlphas {
		return fmt.Errorf("invalid id of alpha: %v", id)
	}
	return c.startContainer(c.alphas[id])
}

func (c *LocalCluster) startContainer(dc dnode) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerStart(ctx, dc.cid(), types.ContainerStartOptions{}); err != nil {
		return errors.Wrapf(err, "error starting container [%v]", dc.cname())
	}
	return nil
}

func (c *LocalCluster) Stop() error {
	glog.Infof("stopping cluster with prefix [%v]", c.conf.prefix)
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

func (c *LocalCluster) StopZero(id int) error {
	if id >= c.conf.numZeros {
		return fmt.Errorf("invalid id of zero: %v", id)
	}
	return c.stopContainer(c.zeros[id])
}

func (c *LocalCluster) StopAlpha(id int) error {
	if id >= c.conf.numAlphas {
		return fmt.Errorf("invalid id of alpha: %v", id)
	}
	return c.stopContainer(c.alphas[id])
}

func (c *LocalCluster) stopContainer(dc dnode) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerStop(ctx, dc.cid(), &stopTimeout); err != nil {
		return errors.Wrapf(err, "error stopping container [%v]", dc.cname())
	}
	return nil
}

func (c *LocalCluster) Cleanup() {
	glog.Infof("cleaning up cluster with prefix [%v]", c.conf.prefix)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	for _, conn := range c.conns {
		if err := conn.Close(); err != nil {
			glog.Warningf("error closing connection: %v", err)
		}
	}

	ro := types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
	for _, aa := range c.alphas {
		if err := c.dcli.ContainerRemove(ctx, aa.cid(), ro); err != nil {
			glog.Warningf("error removing alpha [%v]: %v", aa.cname(), err)
		}
	}
	for _, zo := range c.zeros {
		if err := c.dcli.ContainerRemove(ctx, zo.cid(), ro); err != nil {
			glog.Warningf("error removing zero [%v]: %v", zo.cname(), err)
		}
	}
	if c.net.id != "" {
		if err := c.dcli.NetworkRemove(ctx, c.net.id); err != nil {
			glog.Warningf("error removing network [%v]: %v", c.net.name, err)
		}
	}
	if err := os.RemoveAll(c.tempBinDir); err != nil {
		glog.Warningf("error while removing temp bin dir: %v", err)
	}
}

func (c *LocalCluster) createContainer(dc dnode) (string, error) {
	cmd := dc.cmd(c)
	image := c.dgraphImage()
	mts, err := dc.mounts(c)
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

func (c *LocalCluster) healthCheck() error {
	glog.Infof("checking health of containers")
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

func (c *LocalCluster) containerHealthCheck(url string) error {
	for i := 0; i < 60; i++ {
		time.Sleep(time.Second)

		resp, err := http.Get(url)
		if err != nil {
			glog.Warningf("error hitting health endpoint [%v], err: [%v]", url, err)
			continue
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				glog.Warningf("error closing response body: %v", err)
			}
		}()

		body, berr := io.ReadAll(resp.Body)
		if berr != nil {
			glog.Warningf("error reading health response body: urL: [%v], err: [%v]", url, err)
		}
		sbody := string(body)

		// zero returns OK in the health check
		if sbody == "OK" {
			return nil
		}

		// For Alpha, we only run alpha with EE features enabled
		if strings.Contains(sbody, `"ee_features"`) {
			if !c.conf.acl || strings.Contains(sbody, `"acl"`) {
				return nil
			}
		}
	}

	return fmt.Errorf("health failed, cluster took too long to come up [%v]", url)
}

// Client returns a client that can talk to any Alpha in the cluster
func (c *LocalCluster) Client() (*dgo.Dgraph, error) {
	if c.client != nil {
		return c.client, nil
	}

	var apiClients []api.DgraphClient
	for i := 0; i < c.conf.numAlphas; i++ {
		url, err := c.alphas[i].alphaURL(c)
		if err != nil {
			return nil, errors.Wrap(err, "error getting health URL")
		}
		conn, err := grpc.Dial(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, errors.Wrap(err, "error connecting to alpha")
		}
		c.conns = append(c.conns, conn)
		apiClients = append(apiClients, api.NewDgraphClient(conn))
	}

	client := dgo.NewDgraphClient(apiClients...)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := client.Login(ctx, defaultUser, defaultPassowrd); err != nil {
		return nil, errors.Wrap(err, "error during login")
	}
	c.client = client

	return client, nil
}

// Upgrades the cluster to the provided dgraph version
func (c *LocalCluster) Upgrade(version string) error {
	if version == c.conf.version {
		return fmt.Errorf("cannot upgrade to the same version")
	}

	// cleanup existing connections
	for _, conn := range c.conns {
		if err := conn.Close(); err != nil {
			glog.Warningf("error closing connection: %v", err)
		}
	}
	c.conns = c.conns[:0]
	c.client = nil

	glog.Infof("upgrading the cluster to [%v] using stop-start", version)
	c.conf.version = version
	if err := c.Stop(); err != nil {
		return err
	}

	if err := c.setupBinary(); err != nil {
		return err
	}
	return c.Start()
}

// AssignUids talks to zero to assign the given number of uids
func (c *LocalCluster) AssignUids(num uint64) error {
	if len(c.zeros) == 0 {
		return errors.New("no zero running")
	}

	url, err := c.zeros[0].assignURL(c)
	if err != nil {
		return err
	}

	resp, err := http.Get(fmt.Sprintf("%v?what=uids&num=%d", url, num))
	if err != nil {
		return errors.Wrapf(err, "error talking to zero [%v]", url)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			glog.Warningf("error closing response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "error reading response body")
	}
	var data struct {
		Errors []struct {
			Message string
			Code    string
		}
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return errors.Wrap(err, "error unmarshaling response")
	}
	if len(data.Errors) > 0 {
		return fmt.Errorf("error received from zero: %v", data.Errors[0].Message)
	}
	return nil
}

// CompareCommits compare given commit with current cluster's commit and return
// true if cluster's commit is ancestor of given commit else return false
func CompareCommits(shaToBeCompare, baseSha string) (bool, error) {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return false, errors.Wrap(err, "error while opening git repo")
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(shaToBeCompare))
	if err != nil {
		return false, errors.Wrap(err, "error while getting commit hash")
	}
	commitToBeCompare, err := repo.CommitObject(*hash)
	if err != nil {
		return false, errors.Wrap(err, fmt.Sprintf("error while getting commit object of hash [%v]", hash))
	}
	hash, err = repo.ResolveRevision(plumbing.Revision(baseSha))
	if err != nil {
		return false, err
	}
	baseCommit, err := repo.CommitObject(*hash)
	if err != nil {
		return false, errors.Wrap(err, fmt.Sprintf("error while getting commit object of hash [%v]", hash))
	}
	isParentCommit, err := baseCommit.IsAncestor(commitToBeCompare)
	if err != nil {
		return false, errors.Wrap(err, "error while comparing commits")
	}
	return isParentCommit, nil
}

func (c *LocalCluster) SkipTest(t *testing.T, commit string) error {
	isParentCommit, err := CompareCommits(commit, c.conf.version)
	if err != nil {
		return err
	}
	if isParentCommit {
		t.Skip("skipping this test")
	}
	return nil
}
