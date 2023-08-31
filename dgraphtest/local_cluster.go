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
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	docker "github.com/docker/docker/client"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dgraph-io/dgo/v230"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/x"
)

// cluster's network struct
type cnet struct {
	id   string
	name string
}

// LocalCluster is a local dgraph cluster
type LocalCluster struct {
	conf           ClusterConfig
	tempBinDir     string
	tempSecretsDir string
	encKeyPath     string

	lowerThanV21     bool
	customTokenizers string

	// resources
	dcli   *docker.Client
	net    cnet
	zeros  []*zero
	alphas []*alpha
}

// UpgradeStrategy is an Enum that defines various upgrade strategies
type UpgradeStrategy int

const (
	BackupRestore UpgradeStrategy = iota
	ExportImport
	InPlace
)

func (u UpgradeStrategy) String() string {
	switch u {
	case BackupRestore:
		return "backup-restore"
	case InPlace:
		return "in-place"
	case ExportImport:
		return "export-import"
	default:
		panic("unknown upgrade strategy")
	}
}

// NewLocalCluster creates a new local dgraph cluster with given configuration
func NewLocalCluster(conf ClusterConfig) (*LocalCluster, error) {
	c := &LocalCluster{conf: conf}
	if err := c.init(); err != nil {
		c.Cleanup(true)
		return nil, err
	}
	return c, nil
}

// init performs the one time setup and sets up the cluster.
func (c *LocalCluster) init() error {
	var err error
	c.dcli, err = docker.NewClientWithOpts(docker.FromEnv, docker.WithAPIVersionNegotiation())
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
		return errors.Wrap(err, "error while creating tempBinDir")
	}
	log.Printf("[INFO] tempBinDir: %v", c.tempBinDir)
	c.tempSecretsDir, err = os.MkdirTemp("", c.conf.prefix)
	if err != nil {
		return errors.Wrap(err, "error while creating tempSecretsDir")
	}
	log.Printf("[INFO] tempSecretsDir: %v", c.tempSecretsDir)

	if err := os.Mkdir(binariesPath, os.ModePerm); err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "error while making binariesPath")
	}

	for _, vol := range c.conf.volumes {
		if err := c.createVolume(vol); err != nil {
			return err
		}
	}

	for i := 0; i < c.conf.numZeros; i++ {
		zo := &zero{id: i}
		zo.containerName = fmt.Sprintf(zeroNameFmt, c.conf.prefix, zo.id)
		zo.aliasName = fmt.Sprintf(zeroAliasNameFmt, zo.id)
		c.zeros = append(c.zeros, zo)
	}

	for i := 0; i < c.conf.numAlphas; i++ {
		aa := &alpha{id: i}
		aa.containerName = fmt.Sprintf(alphaNameFmt, c.conf.prefix, aa.id)
		aa.aliasName = fmt.Sprintf(alphaLNameFmt, aa.id)
		c.alphas = append(c.alphas, aa)
	}

	if err := c.setupSecrets(); err != nil {
		return errors.Wrap(err, "error setting up secrets")
	}

	if err := c.setupBeforeCluster(); err != nil {
		return err
	}
	if err := c.createContainers(); err != nil {
		return err
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

	req := volume.CreateOptions{Driver: "local", Name: name}
	if _, err := c.dcli.VolumeCreate(ctx, req); err != nil {
		return errors.Wrapf(err, "error creating volume [%v]", name)
	}
	return nil
}

func (c *LocalCluster) setupBeforeCluster() error {
	if err := c.setupBinary(); err != nil {
		return errors.Wrapf(err, "error setting up binary")
	}

	higher, err := IsHigherVersion(c.GetVersion(), "v21.03.0")
	if err != nil {
		return errors.Wrapf(err, "error checking if version %s is older than v21.03.0", c.GetVersion())
	}
	c.lowerThanV21 = !higher

	return nil
}

func (c *LocalCluster) createContainers() error {
	for _, zo := range c.zeros {
		cid, err := c.createContainer(zo)
		if err != nil {
			return err
		}
		zo.containerID = cid
	}

	for _, aa := range c.alphas {
		cid, err := c.createContainer(aa)
		if err != nil {
			return err
		}
		aa.containerID = cid
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
	hconf := &container.HostConfig{Mounts: mts, PublishAllPorts: true, PortBindings: dc.bindings(c.conf.portOffset)}
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
	resp, err := c.dcli.ContainerCreate(ctx, cconf, hconf, networkConfig, nil, dc.cname())
	if err != nil {
		return "", errors.Wrapf(err, "error creating container %v", dc.cname())
	}

	return resp.ID, nil
}

func (c *LocalCluster) destroyContainers() error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ro := types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
	for _, zo := range c.zeros {
		if err := c.dcli.ContainerRemove(ctx, zo.cid(), ro); err != nil {
			return errors.Wrapf(err, "error removing zero [%v]", zo.cname())
		}
	}

	for _, aa := range c.alphas {
		if err := c.dcli.ContainerRemove(ctx, aa.cid(), ro); err != nil {
			return errors.Wrapf(err, "error removing alpha [%v]", aa.cname())
		}
	}

	return nil
}

func (c *LocalCluster) Cleanup(verbose bool) {
	if c == nil {
		return
	}

	if verbose {
		if err := c.printAllLogs(); err != nil {
			log.Printf("[WARNING] error printing container logs: %v", err)
		}
		if err := c.printInspectContainers(); err != nil {
			log.Printf("[WARNING] error printing inspect container output: %v", err)
		}
	}

	log.Printf("[INFO] cleaning up cluster with prefix [%v]", c.conf.prefix)
	if err := c.destroyContainers(); err != nil {
		log.Printf("[WARNING] error removing container: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	for _, vol := range c.conf.volumes {
		if err := c.dcli.VolumeRemove(ctx, vol, true); err != nil {
			log.Printf("[WARNING] error removing volume [%v]: %v", vol, err)
		}
	}
	if c.net.id != "" {
		if err := c.dcli.NetworkRemove(ctx, c.net.id); err != nil {
			log.Printf("[WARNING] error removing network [%v]: %v", c.net.name, err)
		}
	}
	if err := os.RemoveAll(c.tempBinDir); err != nil {
		log.Printf("[WARNING] error while removing temp bin dir: %v", err)
	}
	if err := os.RemoveAll(c.tempSecretsDir); err != nil {
		log.Printf("[WARNING] error while removing temp secrets dir: %v", err)
	}
}

func (c *LocalCluster) Start() error {
	log.Printf("[INFO] starting cluster with prefix [%v]", c.conf.prefix)
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
	return c.HealthCheck(false)
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
	log.Printf("[INFO] stopping cluster with prefix [%v]", c.conf.prefix)
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

	stopTimeout := 30 // in seconds
	o := container.StopOptions{Timeout: &stopTimeout}
	if err := c.dcli.ContainerStop(ctx, dc.cid(), o); err != nil {
		// Force kill the container if timeout exceeded
		if strings.Contains(err.Error(), "context deadline exceeded") {
			_ = c.dcli.ContainerKill(ctx, dc.cid(), "KILL")
			return nil
		}
		return errors.Wrapf(err, "error stopping container [%v]", dc.cname())
	}
	return nil
}

func (c *LocalCluster) KillAlpha(id int) error {
	if id >= c.conf.numAlphas {
		return fmt.Errorf("invalid id of alpha: %v", id)
	}
	return c.killContainer(c.alphas[id])
}

func (c *LocalCluster) killContainer(dc dnode) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.dcli.ContainerKill(ctx, dc.cid(), "SIGKILL"); err != nil {
		return errors.Wrapf(err, "error killing container [%v]", dc.cname())
	}
	return nil
}

func (c *LocalCluster) HealthCheck(zeroOnly bool) error {
	log.Printf("[INFO] checking health of containers")
	for _, zo := range c.zeros {
		url, err := zo.healthURL(c)
		if err != nil {
			return errors.Wrap(err, "error getting health URL")
		}
		if err := c.containerHealthCheck(url); err != nil {
			return err
		}
		log.Printf("[INFO] container [%v] passed health check", zo.containerName)

		if err := c.checkDgraphVersion(zo.containerName); err != nil {
			return err
		}
	}
	if zeroOnly {
		return nil
	}

	for _, aa := range c.alphas {
		url, err := aa.healthURL(c)
		if err != nil {
			return errors.Wrap(err, "error getting health URL")
		}
		if err := c.containerHealthCheck(url); err != nil {
			return err
		}
		log.Printf("[INFO] container [%v] passed health check", aa.containerName)

		if err := c.checkDgraphVersion(aa.containerName); err != nil {
			return err
		}
	}
	return nil
}

func (c *LocalCluster) containerHealthCheck(url string) error {
	for i := 0; i < 60; i++ {
		time.Sleep(waitDurBeforeRetry)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			log.Printf("[WARNING] error building req for endpoint [%v], err: [%v]", url, err)
			continue
		}
		body, err := doReq(req)
		if err != nil {
			log.Printf("[WARNING] error hitting health endpoint [%v], err: [%v]", url, err)
			continue
		}
		resp := string(body)

		// zero returns OK in the health check
		if resp == "OK" {
			return nil
		}

		// For Alpha, we always run alpha with EE features enabled
		if !strings.Contains(resp, `"ee_features"`) {
			continue
		}
		if c.conf.acl && !strings.Contains(resp, `"acl"`) {
			continue
		}
		if err := c.waitUntilLogin(); err != nil {
			return err
		}
		if err := c.waitUntilGraphqlHealthCheck(); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("health failed, cluster took too long to come up [%v]", url)
}

func (c *LocalCluster) waitUntilLogin() error {
	if !c.conf.acl {
		return nil
	}

	client, cleanup, err := c.Client()
	if err != nil {
		return errors.Wrap(err, "error setting up a client")
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	for i := 0; i < 10; i++ {
		err := client.Login(ctx, DefaultUser, DefaultPassword)
		if err == nil {
			log.Printf("[INFO] login succeeded")
			return nil
		}
		log.Printf("[WARNING] error trying to login: %v", err)
		time.Sleep(waitDurBeforeRetry)
	}
	return errors.New("error during login")
}

func (c *LocalCluster) waitUntilGraphqlHealthCheck() error {
	hc, err := c.HTTPClient()
	if err != nil {
		return errors.Wrap(err, "error creating http client while graphql health check")
	}
	if c.conf.acl {
		if err := hc.LoginIntoNamespace(DefaultUser, DefaultPassword, x.GalaxyNamespace); err != nil {
			return errors.Wrap(err, "error during login while graphql health check")
		}
	}

	for i := 0; i < 10; i++ {
		// we do this because before v21, we used to propose the initial schema to the cluster.
		// This results in schema being applied and indexes being built which could delay alpha
		// starting to serve graphql schema.
		err := hc.DeleteUser("nonexistent")
		if err == nil {
			log.Printf("[INFO] graphql health check succeeded")
			return nil
		} else if strings.Contains(err.Error(), "this indicates a resolver or validation bug") {
			time.Sleep(waitDurBeforeRetry)
			continue
		} else {
			return errors.Wrapf(err, "error during graphql health check")
		}
	}

	return errors.New("error during graphql health check")
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
			log.Printf("[WARNING] error closing response body: %v", err)
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

// Upgrades the cluster to the provided dgraph version
func (c *LocalCluster) Upgrade(version string, strategy UpgradeStrategy) error {
	if version == c.conf.version {
		return fmt.Errorf("cannot upgrade to the same version")
	}

	log.Printf("[INFO] upgrading the cluster from [%v] to [%v] using [%v]", c.conf.version, version, strategy)
	switch strategy {
	case BackupRestore:
		hc, err := c.HTTPClient()
		if err != nil {
			return err
		}
		if c.conf.acl {
			if err := hc.LoginIntoNamespace(DefaultUser, DefaultPassword, x.GalaxyNamespace); err != nil {
				return errors.Wrapf(err, "error during login before upgrade")
			}
		}
		if err := hc.Backup(c, true, DefaultBackupDir); err != nil {
			return errors.Wrap(err, "error taking backup during upgrade")
		}
		if err := c.Stop(); err != nil {
			return err
		}
		c.conf.version = version
		if err := c.recreateContainers(); err != nil {
			return err
		}
		if err := c.Start(); err != nil {
			return err
		}

		var encPath string
		if c.conf.encryption {
			encPath = encKeyMountPath
		}
		hc, err = c.HTTPClient()
		if err != nil {
			return errors.Wrapf(err, "error creating HTTP client after upgrade")
		}
		if c.conf.acl {
			if err := hc.LoginIntoNamespace(DefaultUser, DefaultPassword, x.GalaxyNamespace); err != nil {
				return errors.Wrapf(err, "error during login after upgrade")
			}
		}
		if err := hc.Restore(c, DefaultBackupDir, "", 0, 1, encPath); err != nil {
			return errors.Wrap(err, "error doing restore during upgrade")
		}
		if err := WaitForRestore(c); err != nil {
			return errors.Wrap(err, "error waiting for restore to complete")
		}
		return nil

	case ExportImport:
		hc, err := c.HTTPClient()
		if err != nil {
			return err
		}
		if c.conf.acl {
			if err := hc.LoginIntoNamespace(DefaultUser, DefaultPassword, x.GalaxyNamespace); err != nil {
				return errors.Wrapf(err, "error during login before upgrade")
			}
		}
		// using -1 as namespace exports all the namespaces
		if err := hc.Export(DefaultExportDir, -1); err != nil {
			return errors.Wrap(err, "error taking export during upgrade")
		}
		if err := c.Stop(); err != nil {
			return err
		}
		c.conf.version = version
		if err := c.recreateContainers(); err != nil {
			return err
		}
		if err := c.Start(); err != nil {
			return err
		}
		if err := c.LiveLoadFromExport(DefaultExportDir); err != nil {
			return errors.Wrap(err, "error doing import using live loader")
		}
		return nil

	case InPlace:
		if err := c.Stop(); err != nil {
			return err
		}
		c.conf.version = version
		if err := c.setupBeforeCluster(); err != nil {
			return err
		}
		return c.Start()

	default:
		return errors.New("unknown upgrade strategy")
	}
}

func (c *LocalCluster) recreateContainers() error {
	if err := c.destroyContainers(); err != nil {
		return errors.Wrapf(err, "error while recreaing containers")
	}

	if err := c.setupBeforeCluster(); err != nil {
		return errors.Wrap(err, "error while setupBeforeCluster")
	}

	if err := c.createContainers(); err != nil {
		return errors.Wrapf(err, "error while creating containers")
	}

	return nil
}

// Client returns a grpc client that can talk to any Alpha in the cluster
func (c *LocalCluster) Client() (*GrpcClient, func(), error) {
	// TODO(aman): can we cache the connections?
	var apiClients []api.DgraphClient
	var conns []*grpc.ClientConn
	for i := 0; i < c.conf.numAlphas; i++ {
		url, err := c.alphas[i].alphaURL(c)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error getting health URL")
		}
		conn, err := grpc.Dial(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, nil, errors.Wrap(err, "error connecting to alpha")
		}
		conns = append(conns, conn)
		apiClients = append(apiClients, api.NewDgraphClient(conn))
	}

	client := dgo.NewDgraphClient(apiClients...)
	cleanup := func() {
		for _, conn := range conns {
			if err := conn.Close(); err != nil {
				log.Printf("[WARNING] error closing connection: %v", err)
			}
		}
	}
	return &GrpcClient{Dgraph: client}, cleanup, nil
}

// HTTPClient creates an HTTP client
func (c *LocalCluster) HTTPClient() (*HTTPClient, error) {
	adminURL, err := c.serverURL("alpha", "/admin")
	if err != nil {
		return nil, err
	}
	graphqlURL, err := c.serverURL("alpha", "/graphql")
	if err != nil {
		return nil, err
	}
	licenseURL, err := c.serverURL("zero", "/enterpriseLicense")
	if err != nil {
		return nil, err
	}
	stateURL, err := c.serverURL("zero", "/state")
	if err != nil {
		return nil, err
	}
	dqlURL, err := c.dqlURL()
	if err != nil {
		return nil, err
	}

	return &HTTPClient{
		adminURL:   adminURL,
		graphqlURL: graphqlURL,
		licenseURL: licenseURL,
		stateURL:   stateURL,
		dqlURL:     dqlURL,
	}, nil
}

// serverURL returns url to the 'server' 'endpoint'
func (c *LocalCluster) serverURL(server, endpoint string) (string, error) {
	pubPort, err := publicPort(c.dcli, c.alphas[0], alphaHttpPort)
	if server == "zero" {
		pubPort, err = publicPort(c.dcli, c.zeros[0], zeroHttpPort)
	}
	if err != nil {
		return "", err
	}
	url := "http://localhost:" + pubPort + endpoint
	return url, nil
}

// dqlURL returns url to the dql query endpoint
func (c *LocalCluster) dqlURL() (string, error) {
	publicPort, err := publicPort(c.dcli, c.alphas[0], alphaHttpPort)
	if err != nil {
		return "", err
	}
	url := "http://localhost:" + publicPort + "/query"
	return url, nil
}

// AlphasHealth returns response of health endpoint for all alphas
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

// AlphasLogs returns logs of all the alpha containers
func (c *LocalCluster) AlphasLogs() ([]string, error) {
	alphasLogs := make([]string, 0, len(c.alphas))
	for _, aa := range c.alphas {
		alphaLogs, err := c.getLogs(aa.containerID)
		if err != nil {
			return nil, err
		}
		alphasLogs = append(alphasLogs, alphaLogs)
	}
	return alphasLogs, nil
}

// AssignUids talks to zero to assign the given number of uids
func (c *LocalCluster) AssignUids(_ *dgo.Dgraph, num uint64) error {
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

// GetVersion returns the version of dgraph the cluster is running
func (c *LocalCluster) GetVersion() string {
	return c.conf.version
}

func (c *LocalCluster) printAllLogs() error {
	log.Printf("[INFO] all logs for cluster with prefix [%v] are below!", c.conf.prefix)
	var finalErr error
	for _, zo := range c.zeros {
		if err := c.printLogs(zo.containerName); err != nil {
			finalErr = fmt.Errorf("%v; %v", finalErr, err)
		}
	}
	for _, aa := range c.alphas {
		if err := c.printLogs(aa.containerName); err != nil {
			finalErr = fmt.Errorf("%v; %v", finalErr, err)
		}
	}
	return finalErr
}

func (c *LocalCluster) printLogs(containerID string) error {
	logsData, err := c.getLogs(containerID)
	if err != nil {
		return err
	}

	log.Printf("[INFO] ======== LOGS for CONTAINER [%v] ========", containerID)
	log.Println(logsData)
	return nil
}

func (c *LocalCluster) checkDgraphVersion(containerID string) error {
	if c.GetVersion() == localVersion {
		return nil
	}

	contLogs, err := c.getLogs(containerID)
	if err != nil {
		return errors.Wrapf(err, "error during checkDgraphVersion for container [%v]", containerID)
	}

	// During in-place upgrade, container remains same but logs have version string twice
	// once for old version, once for new. Want new version string. Look bottom-up using
	// LastIndex to get latest version's string.
	index := strings.LastIndex(contLogs, "Commit SHA-1     : ")
	running := strings.Fields(contLogs[index : index+70])[3] // 70 is arbitrary
	chash, err := getHash(c.GetVersion())
	if err != nil {
		return errors.Wrapf(err, "error while getting hash for %v", c.GetVersion())
	}
	rhash, err := getHash(running)
	if err != nil {
		return errors.Wrapf(err, "error while getting hash for %v", running)
	}
	if chash != rhash {
		return errors.Errorf("found different dgraph version [%v] than expected [%v]", rhash, chash)
	}
	return nil
}

func (c *LocalCluster) getLogs(containerID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	opts := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Details:    true,
	}
	ro, err := c.dcli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return "", errors.Wrapf(err, "error collecting logs for %v", containerID)
	}
	defer func() {
		if err := ro.Close(); err != nil {
			log.Printf("[WARNING] error in closing reader for [%v]: %v", containerID, err)
		}
	}()

	data, err := io.ReadAll(ro)
	if err != nil {
		log.Printf("[WARNING] error in reading logs for [%v]: %v", containerID, err)
	}
	return string(data), nil
}

func (c *LocalCluster) printInspectContainers() error {
	log.Printf("[INFO] inspecting all container for cluster with prefix [%v]", c.conf.prefix)
	var finalErr error
	for _, zo := range c.zeros {
		if err := c.printInspectFor(zo.containerName); err != nil {
			finalErr = fmt.Errorf("%v; %v", finalErr, err)
		}
	}
	for _, aa := range c.alphas {
		if err := c.printInspectFor(aa.containerName); err != nil {
			finalErr = fmt.Errorf("%v; %v", finalErr, err)
		}
	}
	return finalErr
}

func (c *LocalCluster) printInspectFor(containerID string) error {
	inspectData, err := c.inspectContainer(containerID)
	if err != nil {
		return err
	}

	log.Printf("[INFO] ======== INSPECTING CONTAINER [%v] ========", containerID)
	log.Println(inspectData)
	return nil
}

func (c *LocalCluster) inspectContainer(containerID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	_, raw, err := c.dcli.ContainerInspectWithRaw(ctx, containerID, true)
	if err != nil {
		return "", errors.Wrapf(err, "error inspecting container %v", containerID)
	}
	return string(raw), nil
}

func (c *LocalCluster) setupSecrets() error {
	if c.conf.encryption {
		// use this key because some of the data is already encrypted using this key.
		encKey := []byte("1234567890123456")
		c.encKeyPath = filepath.Join(c.tempSecretsDir, encKeyFile)
		if err := os.WriteFile(c.encKeyPath, encKey, 0600); err != nil {
			return err
		}
	}

	if c.conf.acl {
		aclSecretPath := filepath.Join(c.tempSecretsDir, aclKeyFile)
		if err := generateACLSecret(c.conf.aclAlg, aclSecretPath); err != nil {
			return err
		}
	}

	return nil
}

func generateACLSecret(alg jwt.SigningMethod, pathToFile string) error {
	if alg == nil {
		return randomData(32*8, pathToFile)
	}

	switch alg.Alg() {
	case "HS256", "HS384", "HS512":
		return randomData(64*8, pathToFile)
	case "RS256", "RS384", "RS512", "PS256", "PS384", "PS512":
		return rsaPem(2048, pathToFile)
	case "ES256":
		return ecdsaPem("prime256v1", pathToFile)
	case "ES384":
		return ecdsaPem("secp384r1", pathToFile)
	case "ES512":
		return ecdsaPem("secp521r1", pathToFile)
	case "ES256K":
		return ecdsaPem("secp256k1", pathToFile)
	case "EdDSA":
		return ed25519Pem(pathToFile)
	default:
		return errors.Errorf("unsupported ACL algorithm: %v", alg.Alg())
	}
}

func randomData(bits int, pathToFile string) error {
	return runOpennssl("openssl", "rand", "-out", pathToFile, strconv.Itoa(bits/8))
}

func rsaPem(bits int, pathToFile string) error {
	return runOpennssl("openssl", "genrsa", "-out", pathToFile, strconv.Itoa(bits))
}

func ecdsaPem(alg string, pathToFile string) error {
	return runOpennssl("openssl", "ecparam", "-name", alg, "-genkey", "-noout", "-out", pathToFile)
}

func ed25519Pem(pathToFile string) error {
	return runOpennssl("openssl", "genpkey", "-algorithm", "Ed25519", "-out", pathToFile)
}

func runOpennssl(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "### failed to run openssl cmd [%v] ###\n%v", cmd, string(out))
	}
	return nil
}

func (c *LocalCluster) GeneratePlugins(raceEnabled bool) error {
	_, curr, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("error while getting current file")
	}
	var soFiles []string
	for i, src := range []string{
		"../testutil/custom_plugins/anagram/main.go",
		"../testutil/custom_plugins/cidr/main.go",
		"../testutil/custom_plugins/factor/main.go",
		"../testutil/custom_plugins/rune/main.go",
	} {
		so := c.tempBinDir + "/plugins/" + strconv.Itoa(i) + ".so"
		log.Printf("compiling plugin: src=%q so=%q\n", src, so)
		opts := []string{"build"}
		if raceEnabled {
			opts = append(opts, "-race")
		}
		opts = append(opts, "-buildmode=plugin", "-o", so, src)
		os.Setenv("GOOS", "linux")
		os.Setenv("GOARCH", "amd64")
		cmd := exec.Command("go", opts...)
		cmd.Dir = filepath.Dir(curr)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Error: %v\n", err)
			log.Printf("Output: %v\n", string(out))
			return err
		}
		absSO, err := filepath.Abs(so)
		if err != nil {
			log.Printf("Error: %v\n", err)
			return err
		}
		soFiles = append(soFiles, absSO)
	}

	sofiles := strings.Join(soFiles, ",")
	c.customTokenizers = strings.ReplaceAll(sofiles, c.tempBinDir, goBinMountPath)
	log.Printf("plugin build completed. Files are: %s\n", sofiles)

	return nil
}
