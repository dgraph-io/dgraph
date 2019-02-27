package main

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/dgraph-io/dgraph/x"
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
		Source:   "$GOPATH/bin",
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
	i.Command += fmt.Sprintf(" --replicas=%d", Opts.NumAlphas/Opts.NumGroups)
	i.Command += " --logtostderr"
	if idx == 1 {
		i.Command += fmt.Sprintf(" --bindall")
	} else {
		i.Command += fmt.Sprintf(" --peer=%s:%d", name(namePfx, 1), zeroBasePort)
	}

	return i
}
func getAlpha(idx int) Instance {
	baseOffset := 0
	if Opts.TestPortRange {
		baseOffset += 100
	}

	namePfx := "alpha"
	svcName := name(namePfx, idx)
	itnlPort := alphaBasePort + baseOffset + idx - 1
	grpcPort := itnlPort + 1000
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
	i.Command = fmt.Sprintf("/gobin/dgraph alpha -o %d", baseOffset+idx-1)
	i.Command += fmt.Sprintf(" --my=%s:%d", svcName, itnlPort)
	i.Command += fmt.Sprintf(" --lru_mb=%d", Opts.LruSizeMB)
	i.Command += fmt.Sprintf(" --zero=zero1:%d", zeroBasePort)
	i.Command += " --logtostderr"
	i.Command += " --whitelist=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	if Opts.EnterpriseMode {
		i.Command += " --enterprise_features"
	}

	return i
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "compose: %v", err)
	os.Exit(1)
}

func main() {
	flag.CommandLine = flag.NewFlagSet("compose", flag.ExitOnError)
	flag.IntVar(&Opts.NumZeros, "num_zeros", 1,
		"number of zeros in dgraph cluster")
	flag.IntVar(&Opts.NumAlphas, "num_alphas", 1,
		"number of alphas in dgraph cluster")
	flag.IntVar(&Opts.NumGroups, "num_groups", 1,
		"number of groups in dgraph cluster")
	flag.IntVar(&Opts.LruSizeMB, "lru_mb", 1024,
		"approximate size of LRU cache")
	flag.BoolVar(&Opts.EnterpriseMode, "enterprise", false,
		"enable enterprise features in alphas")
	flag.BoolVar(&Opts.TestPortRange, "test_ports", true,
		"use port range expected by regression tests in alphas ")
	flag.Parse()

	// Do some sanity checks.
	if Opts.NumZeros < 1 || Opts.NumZeros > 99 {
		fatal(fmt.Errorf("number of zeros must be 1-99"))
	}
	if Opts.NumAlphas < 1 || Opts.NumAlphas > 99 {
		fatal(fmt.Errorf("number of alphas must be 1-99"))
	}
	if Opts.LruSizeMB < 1024 {
		fatal(fmt.Errorf("LRU cache size must be >= 1024 MB"))
	}
	if Opts.NumAlphas%Opts.NumGroups != 0 {
		fatal(fmt.Errorf("%d alphas do not divide evenly into %d groups\n",
			Opts.NumAlphas, Opts.NumGroups))
	}
	if (Opts.NumAlphas/Opts.NumGroups)%2 == 0 {
		fatal(fmt.Errorf("groups with even number (%d) of nodes are not recommended\n",
			Opts.NumAlphas/Opts.NumGroups))
	}

	services := make(map[string]Instance)

	for i := 1; i <= Opts.NumZeros; i++ {
		instance := getZero(i)
		services[instance.ContainerName] = instance
	}

	for i := 1; i <= Opts.NumAlphas; i++ {
		instance := getAlpha(i)
		services[instance.ContainerName] = instance
	}

	c := Config{
		Version:  "3.5",
		Services: services,
	}

	out, err := yaml.Marshal(c)
	x.Check(err)
	fmt.Printf("%s", out)
}
