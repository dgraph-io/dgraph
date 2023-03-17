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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	docker "github.com/docker/docker/client"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
)

var (
	requestTimeout = 90 * time.Second
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
	zeros  []*zero
	alphas []*alpha
}

type UpgradeStrategy int

const (
	BackupRestore UpgradeStrategy = iota
	StopStart
)

func (u UpgradeStrategy) String() string {
	switch u {
	case BackupRestore:
		return "backup-restore"
	case StopStart:
		return "stop-start"
	default:
		panic("unknown upgrade strategy")
	}
}

func NewLocalCluster(conf ClusterConfig) (*LocalCluster, error) {
	c := &LocalCluster{conf: conf}
	if err := c.init(); err != nil {
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
		return err
	}
	c.tempBinDir, err = os.MkdirTemp("", c.conf.prefix)
	if err != nil {
		return errors.Wrap(err, "error while creating temp dir")
	}
	if err := os.Mkdir(binDir, os.ModePerm); err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "error while making binDir")
	}

	for _, vol := range c.conf.volumes {
		volname := fmt.Sprintf(volNameFmt, c.conf.prefix, vol)
		if err := c.createVolume(volname); err != nil {
			return err
		}
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

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	network, err := c.dcli.NetworkCreate(ctx, c.net.name, opts)
	if err != nil {
		return errors.Wrap(err, "error creating network")
	}
	c.net.id = network.ID

	return nil
}

func (c *LocalCluster) createVolume(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req := volume.VolumesCreateBody{Driver: "local", Name: name}
	if _, err := c.dcli.VolumeCreate(ctx, req); err != nil {
		return errors.Wrapf(err, "error creating volume [%v]", name)
	}
	return nil
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
	for _, vol := range c.conf.volumes {
		volname := fmt.Sprintf(volNameFmt, c.conf.prefix, vol)
		if err := c.dcli.VolumeRemove(ctx, volname, true); err != nil {
			glog.Warningf("error removing volume [%v]: %v", vol, err)
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

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			glog.Warningf("error building req for endpoint [%v], err: [%v]", url, err)
			continue
		}
		body, err := doReq(req)
		if err != nil {
			glog.Warningf("error hitting health endpoint [%v], err: [%v]", url, err)
			continue
		}
		resp := string(body)

		// zero returns OK in the health check
		if resp == "OK" {
			return nil
		}

		// For Alpha, we always run alpha with EE features enabled
		if strings.Contains(resp, `"ee_features"`) {
			if !c.conf.acl || strings.Contains(resp, `"acl"`) {
				return nil
			}
		}
	}

	return fmt.Errorf("health failed, cluster took too long to come up [%v]", url)
}

var client *http.Client = &http.Client{
	Timeout: requestTimeout,
}

func doReq(req *http.Request) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error performing HTTP request")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			glog.Warningf("error closing response body: %v", err)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading response body: url: [%v], err: [%v]", req.URL, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got non 200 resp: %v", string(respBody))
	}
	return respBody, nil
}

func (c *LocalCluster) recreateContainers() error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ro := types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
	for _, zo := range c.zeros {
		if err := c.dcli.ContainerRemove(ctx, zo.cid(), ro); err != nil {
			return errors.Wrapf(err, "error removing zero [%v]", zo.cname())
		}
		cid, err := c.createContainer(zo)
		if err != nil {
			return err
		}
		zo.containerID = cid
	}

	for _, aa := range c.alphas {
		if err := c.dcli.ContainerRemove(ctx, aa.cid(), ro); err != nil {
			return errors.Wrapf(err, "error removing alpha [%v]", aa.cname())
		}
		cid, err := c.createContainer(aa)
		if err != nil {
			return err
		}
		aa.containerID = cid
	}

	return nil
}

// Upgrades the cluster to the provided dgraph version
func (c *LocalCluster) Upgrade(version string, strategy UpgradeStrategy) error {
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

	glog.Infof("upgrading the cluster from [%v] to [%v] using [%v]", c.conf.version, version, strategy)
	switch strategy {
	case BackupRestore:
		if err := Backup(c, true, DefaultBackupDir); err != nil {
			return errors.Wrap(err, "error taking backup during upgrade")
		}
		if err := c.Stop(); err != nil {
			return err
		}
		c.conf.version = version
		if err := c.setupBinary(); err != nil {
			return err
		}
		if err := c.recreateContainers(); err != nil {
			return err
		}
		if err := c.Start(); err != nil {
			return err
		}
		if err := Restore(c, DefaultBackupDir, "", 0, 1, encKeyMountPath); err != nil {
			return errors.Wrap(err, "error doing restore during upgrade")
		}
		if err := WaitForRestore(c); err != nil {
			return errors.Wrap(err, "error waiting for restore to complete")
		}
		return nil

	case StopStart:
		if err := c.Stop(); err != nil {
			return err
		}
		c.conf.version = version
		if err := c.setupBinary(); err != nil {
			return err
		}
		return c.Start()

	default:
		return errors.New("unknown upgrade strategy")
	}
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
	if c.conf.acl {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		var err error
		for i := 0; i < 3; i++ {
			if err = client.Login(ctx, defaultUser, defaultPassowrd); err == nil {
				break
			}
			glog.Warningf("error trying to login: %v", err)
			time.Sleep(waitDurBeforeRetry)
		}
		if err != nil {
			return nil, errors.Wrap(err, "error during login")
		}
	}
	c.client = client

	return client, nil
}

func (c *LocalCluster) AlphasHealth() ([]string, error) {
	if len(c.alphas) == 0 {
		return nil, fmt.Errorf("alpha not running")
	}

	healths := make([]string, 0, c.conf.numAlphas)
	for _, a := range c.alphas {
		url, err := a.healthURL(c)
		if err != nil {
			return nil, errors.Wrap(err, "error getting health URL")
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "error building req for endpoint [%v]", url)
		}
		h, err := doReq(req)
		if err != nil {
			return nil, errors.Wrap(err, "error getting health")
		}
		healths = append(healths, string(h))
	}

	return healths, nil
}

func httpLogin(endpoint string) (string, error) {
	q := `mutation login($userId: String, $password: String, $namespace: Int) {
		login(userId: $userId, password: $password, namespace: $namespace) {
			response {
				accessJWT
				refreshJWT
			}
		}
	}`

	gqlParams := graphQLParams{
		Query: q,
		Variables: map[string]interface{}{
			"userId":   defaultUser,
			"password": defaultPassowrd,
		},
	}
	body, err := json.Marshal(gqlParams)
	if err != nil {
		return "", errors.Wrapf(err, "unable to marshal body")
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", errors.Wrapf(err, "unable to create request")
	}
	req.Header.Set("Content-Type", "application/json")

	respBody, err := doReq(req)
	if err != nil {
		return "", errors.Wrapf(err, "error performing login request")
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return "", errors.Wrap(err, "error unmarshalling GQL response")
	}
	if len(gqlResp.Errors) > 0 {
		return "", errors.Wrapf(gqlResp.Errors, "error while running admin query")
	}

	var r struct {
		Login struct {
			Response struct {
				AccessJWT  string
				RefreshJwt string
			}
		}
	}
	if err := json.Unmarshal(gqlResp.Data, &r); err != nil {
		return "", errors.Wrap(err, "error unmarshalling response into object")
	}
	if r.Login.Response.AccessJWT == "" {
		return "", errors.Errorf("no access JWT found in the response")
	}
	return r.Login.Response.AccessJWT, nil
}

func (c *LocalCluster) AdminPost(body []byte) ([]byte, error) {
	publicPort, err := publicPort(c.dcli, c.alphas[0], alphaHttpPort)
	if err != nil {
		return nil, err
	}
	url := "http://localhost:" + publicPort + "/admin"

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrapf(err, "error building req for endpoint [%v]", url)
	}
	req.Header.Add("Content-Type", "application/json")

	if c.conf.acl {
		token, err := httpLogin(url)
		if err != nil {
			return nil, err
		}
		req.Header.Add("X-Dgraph-AccessToken", token)
	}

	return doReq(req)
}

// AssignUids talks to zero to assign the given number of uids
func (c *LocalCluster) AssignUids(num uint64) error {
	if len(c.zeros) == 0 {
		return errors.New("no zero running")
	}

	baseURL, err := c.zeros[0].assignURL(c)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%v?what=uids&num=%d", baseURL, num)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrapf(err, "error building req for endpoint [%v]", url)
	}
	body, err := doReq(req)
	if err != nil {
		return err
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

func (c *LocalCluster) GetVersion() string {
	return c.conf.version
}
