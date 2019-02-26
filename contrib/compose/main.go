package main

import (
	"fmt"
	"os"
	"path"

	"github.com/dgraph-io/dgraph/x"
	yaml "gopkg.in/yaml.v2"
)

type Volume struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool `yaml:"read_only"`
}

type Instance struct {
	Image         string
	ContainerName string   `yaml:"container_name"`
	WorkingDir    string   `yaml:"working_dir"`
	DependsOn     []string `yaml:"depends_on,omitempty"`
	Labels        map[string]string
	Ports         []string
	Volumes       []Volume
	Command       string
}

type Config struct {
	Version  string
	Services map[string]Instance
}

type State struct {
	ZeroIdx  int
	AlphaIdx int
}

var state State

const zeroBasePort int = 5080
const alphaBasePort int = 7080

func name(prefix string, idx int) string {
	return fmt.Sprintf("%s%d", prefix, idx)
}

func toString(i int) string {
	return fmt.Sprintf("%d", i)
}

func toPort(i int) string {
	return toString(i) + ":" + toString(i)
}

func binVolume() Volume {
	return Volume{
		Type:     "bind",
		Source:   path.Join(os.Getenv("GOPATH"), "bin"),
		Target:   "/gobin",
		ReadOnly: true,
	}
}

func getZero() Instance {
	state.ZeroIdx++
	idx := state.ZeroIdx

	var i Instance
	i.Image = "dgraph/dgraph:latest"
	i.ContainerName = name("dg0.", idx)
	i.WorkingDir = fmt.Sprintf("/data/%s", i.ContainerName)
	if idx > 1 {
		i.DependsOn = append(i.DependsOn, name("dg0.", idx-1))
	}
	i.Labels = map[string]string{"cluster": "test"}

	grpc := toPort(zeroBasePort + idx - 1)
	http := toPort(zeroBasePort + 1000 + idx - 1)
	i.Ports = []string{grpc, http}

	i.Volumes = append(i.Volumes, binVolume())
	i.Command = fmt.Sprintf("/gobin/dgraph zero -o %d", 100+idx-1)
	return i
}
func getAlpha() Instance {
	state.AlphaIdx++
	idx := state.AlphaIdx

	var i Instance
	i.Image = "dgraph/dgraph:latest"
	i.ContainerName = name("dg", idx)
	i.WorkingDir = fmt.Sprintf("/data/%s", i.ContainerName)
	if idx > 1 {
		i.DependsOn = append(i.DependsOn, name("dg", idx-1))
	}
	i.Labels = map[string]string{"cluster": "test"}

	http := toPort(alphaBasePort + 1100 + idx - 1)
	grpc := toPort(alphaBasePort + 2100 + idx - 1)
	i.Ports = []string{http, grpc}

	i.Volumes = append(i.Volumes, binVolume())
	i.Command = fmt.Sprintf("/gobin/dgraph alpha --lru_mb=1024 -o %d", 100+idx-1)
	return i
}

func main() {
	c := Config{
		Version: "3.5",
		Services: map[string]Instance{
			"dg0.1": getZero(),
			"dg1":   getAlpha(),
		},
	}

	out, err := yaml.Marshal(c)
	x.Check(err)
	fmt.Printf("%s\n", out)
}
