package alpha

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/golang/glog"
)

type Docker struct {
	client *client.Client
	auth   string
}

type Container struct {
	id   string
	name string
	port int
}

func NewDockerClient(ctx context.Context, registry string, user string, password string) (*Docker, error) {
	c, err := client.NewClientWithOpts(
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}

	var authConfig = types.AuthConfig{
		Username:      user,
		Password:      password,
		ServerAddress: registry,
	}
	authConfigBytes, _ := json.Marshal(authConfig)
	authConfigEncoded := base64.URLEncoding.EncodeToString(authConfigBytes)

	return &Docker{client: c, auth: authConfigEncoded}, nil
}

func (d *Docker) PullImage(ctx context.Context, image string) error {
	r, err := d.client.ImagePull(ctx, image, types.ImagePullOptions{RegistryAuth: d.auth})
	if err != nil {
		return err
	}
	io.Copy(os.Stdout, r)

	return nil
}

func (d *Docker) RunContainer(ctx context.Context, image string, suffix int, port int) (*Container, error) {
	containers, err := d.client.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	for _, container := range containers {
		for _, name := range container.Names {
			glog.Info(name)
			glog.Info(strings.HasSuffix(name, fmt.Sprintf("lambda-%d", suffix)))
			if strings.HasSuffix(name, fmt.Sprintf("lambda-%d", suffix)) {
				d.RemoveContainer(ctx, container.ID)
			}
		}
	}

	networkId, err := d.CreateNetwork(ctx)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("lambda-%d", suffix)
	endpoint := make(map[string]*network.EndpointSettings)
	endpoint["dgraph"] = &network.EndpointSettings{NetworkID: networkId}
	container, err := d.client.ContainerCreate(ctx,
		&container.Config{Image: image, Env: []string{"DGRAPH_URL=172.18.0.1"}},
		&container.HostConfig{PortBindings: nat.PortMap{
			nat.Port("8686/tcp"): []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(port)}},
		},
		},
		&network.NetworkingConfig{EndpointsConfig: endpoint}, nil, name)
	if err != nil {
		return nil, err
	}

	c := &Container{
		id:   container.ID,
		name: name,
		port: port,
	}
	return c, d.client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
}

func (d *Docker) RemoveContainer(ctx context.Context, id string) error {
	timeout := time.Second * 30

	if err := d.client.ContainerStop(ctx, id, &timeout); err != nil {
		return err
	}
	if err := d.client.ContainerRemove(ctx, id, types.ContainerRemoveOptions{}); err != nil {
		return err
	}
	return nil
}

func (d *Docker) CreateNetwork(ctx context.Context) (string, error) {

	networks, err := d.client.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return "", err
	}
	for _, network := range networks {
		if network.Name == "dgraph" {
			d.client.NetworkRemove(ctx, network.ID)
		}
	}

	net, err := d.client.NetworkCreate(ctx, "dgraph", types.NetworkCreate{Driver: "bridge", IPAM: &network.IPAM{Config: []network.IPAMConfig{{Subnet: "172.18.0.0/16"}}}})
	if err != nil {
		glog.Info(err)
	}
	return net.ID, nil
}
