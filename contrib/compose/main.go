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

// TODO set these from command line
type Options struct {
	NumZeros       int
	NumAlphas      int
	NumGroups      int
	LruSizeMB      int
	EnterpriseMode bool
	TestPortRange  bool
}

var Opts Options

const zeroBasePort int = 5080
const alphaBasePort int = 7080

func name(prefix string, idx int) string {
	return fmt.Sprintf("%s%02d", prefix, idx)
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

func getZero(idx int) Instance {
	namePfx := "zero"
	svcName := name(namePfx, idx)
	grpcPort := zeroBasePort + idx - 1
	httpPort := grpcPort + 1000

	var i Instance
	i.Image = "dgraph/dgraph:latest"
	i.ContainerName = svcName
	i.WorkingDir = fmt.Sprintf("/data/%s", i.ContainerName)
	if idx > 1 {
		i.DependsOn = append(i.DependsOn, name(namePfx, idx-1))
	}
	i.Labels = map[string]string{"cluster": "test"}

	i.Ports = []string{
		toPort(grpcPort),
		toPort(httpPort),
	}

	i.Volumes = append(i.Volumes, binVolume())
	i.Command = fmt.Sprintf("/gobin/dgraph zero -o %d --idx=%d", idx-1, idx)
	i.Command += fmt.Sprintf(" --my=%s:%d", svcName, grpcPort)
	i.Command += fmt.Sprintf(" --replicas=%d", Opts.NumAlphas)
	i.Command += " --logtostderr"
	if idx == 1 {
		i.Command += fmt.Sprintf(" --bindall")
	} else {
		i.Command += fmt.Sprintf(" --peer=%s:%d", name(namePfx, 1), zeroBasePort)
	}

	return i
}
func getAlpha(idx int) Instance {
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
	Opts = Options{
		NumZeros:  3,
		NumAlphas: 3,
		NumGroups: 2,
		LruSizeMB: 2018,
	}

	services := make(map[string]Instance)
	for i := 1; i <= Opts.NumZeros; i++ {
		instance := getZero(i)
		services[instance.ContainerName] = instance
	}

	c := Config{
		Version:  "3.5",
		Services: services,
		//Services: map[string]Instance{
		//	"dg0.1": getZero(1),
		//	"dg1":   getAlpha(1),
		//},
	}

	out, err := yaml.Marshal(c)
	x.Check(err)
	fmt.Printf("%s\n", out)
}
