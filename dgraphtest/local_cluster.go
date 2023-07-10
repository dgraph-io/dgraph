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
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	docker "github.com/docker/docker/client"
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
	conf       ClusterConfig
	tempBinDir string

	// resources
	dcli     *docker.Client
	net      cnet
	zeros    []*zero
	alphas   []*alpha
	generics []*genericContainer
}

// UpgradeStrategy is an Enum that defines various upgrade strategies
type UpgradeStrategy int

const (
	BackupRestore UpgradeStrategy = iota
	ExportImport
	StopStart
)

func (u UpgradeStrategy) String() string {
	switch u {
	case BackupRestore:
		return "backup-restore"
	case StopStart:
		return "stop-start"
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
	log.Printf("[INFO] tempBinDir: %v", c.tempBinDir)
	if err := os.Mkdir(binDir, os.ModePerm); err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "error while making binDir")
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

	for i := 0; i < c.conf.genContainers; i++ {
		gg := &genericContainer{id: i}
		gg.containerName = "mock" //fmt.Sprintf(genNameFmt, c.conf.prefix, gg.id)
		gg.aliasName = fmt.Sprintf(genNameFmt, gg.id)
		cid, err := c.createGenericContainer(gg)
		if err != nil {
			return err
		}
		gg.containerID = cid
		c.generics = append(c.generics, gg)
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
	resp, err := c.dcli.ContainerCreate(ctx, cconf, hconf, networkConfig, dc.cname())
	if err != nil {
		return "", errors.Wrapf(err, "error creating container %v", dc.cname())
	}

	return resp.ID, nil
}

func dirToTar() io.Reader {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	dir := "/home/siddesh/workspace/Graphql_upgrade_test_suite/dgraph/graphql/e2e/custom_logic/cmd"
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Fatal(err, " :unable to walk through directory")
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			log.Fatal(err, " :unable to get relative path")
		}
		if relPath == "." {
			return nil
		}
		tarHeader, err := tar.FileInfoHeader(info, relPath)
		if err != nil {
			log.Fatal(err, " :unable to create tar header")
		}
		tarHeader.Name = relPath
		err = tw.WriteHeader(tarHeader)
		if err != nil {
			log.Fatal(err, " :unable to write tar header")
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			log.Fatal(err, " :unable to open file")
		}
		_, err = io.Copy(tw, file)
		if err != nil {
			log.Fatal(err, " :unable to copy file to tar writer")
		}
		return nil
	})
	if err != nil {
		log.Fatal(err, " :unable to walk through the directory")
	}
	return bytes.NewReader(buf.Bytes())
}

func (c *LocalCluster) createGenericContainer(dc dnode) (string, error) {
	imgName := "mock-image:latest"
	dockerClient, err := docker.NewEnvClient()
	if err != nil {
		panic(err)
	}

	imageBuildResponse, err := dockerClient.ImageBuild(
		context.Background(),
		dirToTar(),
		types.ImageBuildOptions{
			Context:    dirToTar(),
			Dockerfile: "Dockerfile",
			Tags:       []string{imgName}})
	if err != nil {
		log.Fatal(err, " :unable to build docker image")
	}
	defer imageBuildResponse.Body.Close()
	_, err = io.Copy(os.Stdout, imageBuildResponse.Body)
	if err != nil {
		log.Fatal(err, " :unable to read image build response")
	}

	cconf := &container.Config{Image: imgName, WorkingDir: dc.workingDir(), ExposedPorts: dc.ports()}
	hconf := &container.HostConfig{PublishAllPorts: true, PortBindings: dc.bindings(c.conf.portOffset)}
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
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ro := types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}
	for _, aa := range c.alphas {
		if err := c.dcli.ContainerRemove(ctx, aa.cid(), ro); err != nil {
			log.Printf("[WARNING] error removing alpha [%v]: %v", aa.cname(), err)
		}
	}
	for _, zo := range c.zeros {
		if err := c.dcli.ContainerRemove(ctx, zo.cid(), ro); err != nil {
			log.Printf("[WARNING] error removing zero [%v]: %v", zo.cname(), err)
		}
	}
	for _, gg := range c.generics {
		if err := c.dcli.ContainerRemove(ctx, gg.cid(), ro); err != nil {
			log.Printf("[WARNING] error removing generic [%v]: %v", gg.cname(), err)
		}
	}
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
	for i := 0; i < c.conf.genContainers; i++ {
		if err := c.StartGeneric(i); err != nil {
			return err
		}
	}
	return c.HealthCheck()
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

func (c *LocalCluster) StartGeneric(id int) error {
	if id >= c.conf.genContainers {
		return fmt.Errorf("invalid id of generic container: %v", id)
	}
	return c.startContainer(c.generics[id])
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
	for i := range c.generics {
		if err := c.StopGeneric(i); err != nil {
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

func (c *LocalCluster) StopGeneric(id int) error {
	if id >= c.conf.genContainers {
		return fmt.Errorf("invalid id of generic container: %v", id)
	}
	return c.stopContainer(c.generics[id])
}

func (c *LocalCluster) stopContainer(dc dnode) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	stopTimeout := requestTimeout
	if err := c.dcli.ContainerStop(ctx, dc.cid(), &stopTimeout); err != nil {
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

func (c *LocalCluster) HealthCheck() error {
	log.Printf("[INFO] checking health of containers")
	for i := 0; i < c.conf.numZeros; i++ {
		url, err := c.zeros[i].healthURL(c)
		if err != nil {
			return errors.Wrap(err, "error getting health URL")
		}
		if err := c.containerHealthCheck(url); err != nil {
			return err
		}
		log.Printf("[INFO] container [zero-%v] passed health check", i)
	}
	for i := 0; i < c.conf.numAlphas; i++ {
		url, err := c.alphas[i].healthURL(c)
		if err != nil {
			return errors.Wrap(err, "error getting health URL")
		}
		if err := c.containerHealthCheck(url); err != nil {
			return err
		}
		log.Printf("[INFO] container [alpha-%v] passed health check", i)
	}
	for i := 0; i < c.conf.genContainers; i++ {
		url, err := c.alphas[i].healthURL(c)
		if err != nil {
			return errors.Wrap(err, "error getting health URL")
		}
		if err := c.containerHealthCheck(url); err != nil {
			return err
		}
		log.Printf("[INFO] container [generic-%v] passed health check", i)
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
		if !c.conf.acl {
			return nil
		}
		if !c.conf.acl || !strings.Contains(resp, `"acl"`) {
			continue
		}

		client, cleanup, err := c.Client()
		if err != nil {
			return errors.Wrap(err, "error setting up a client")
		}
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		for i := 0; i < 10; i++ {
			if err = client.Login(ctx, DefaultUser, DefaultPassword); err == nil {
				break
			}
			log.Printf("[WARNING] error trying to login: %v", err)
			time.Sleep(waitDurBeforeRetry)
		}
		if err != nil {
			return errors.Wrap(err, "error during login")
		}
		return nil
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

// needed during upgrade
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

	for _, gg := range c.generics {
		if err := c.dcli.ContainerRemove(ctx, gg.cid(), ro); err != nil {
			return errors.Wrapf(err, "error removing generic [%v]", gg.cname())
		}
		cid, err := c.createContainer(gg)
		if err != nil {
			return err
		}
		gg.containerID = cid
	}

	return nil
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
		if err := c.setupBinary(); err != nil {
			return err
		}
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
		if err := hc.Export(DefaultExportDir); err != nil {
			return errors.Wrap(err, "error taking export during upgrade")
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
		if err := c.LiveLoadFromExport(DefaultExportDir); err != nil {
			return errors.Wrap(err, "error doing import using live loader")
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
	adminURL, err := c.adminURL()
	if err != nil {
		return nil, err
	}
	graphqlURL, err := c.graphqlURL()
	if err != nil {
		return nil, err
	}
	probeGraphqlURL, err := c.probeGraphqlURL()
	if err != nil {
		return nil, err
	}
	return &HTTPClient{adminURL: adminURL, graphqlURL: graphqlURL, probeGraphqlURL: probeGraphqlURL}, nil
}

// adminURL returns url to the graphql admin endpoint
func (c *LocalCluster) adminURL() (string, error) {
	publicPort, err := publicPort(c.dcli, c.alphas[0], alphaHttpPort)
	if err != nil {
		return "", err
	}
	url := "http://localhost:" + publicPort + "/admin"
	return url, nil
}

// graphqlURL returns url to the graphql endpoint
func (c *LocalCluster) graphqlURL() (string, error) {
	publicPort, err := publicPort(c.dcli, c.alphas[0], alphaHttpPort)
	if err != nil {
		return "", err
	}
	url := "http://localhost:" + publicPort + "/graphql"
	return url, nil
}

// probeGraphqlURL returns url to the graphql endpoint
func (c *LocalCluster) probeGraphqlURL() (string, error) {
	publicPort, err := publicPort(c.dcli, c.alphas[0], alphaHttpPort)
	if err != nil {
		return "", err
	}
	url := "http://localhost:" + publicPort + "/probe/graphql"
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
