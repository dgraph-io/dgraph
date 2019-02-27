/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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
package main

import (
	"flag"
	"fmt"
	"math"
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

type ComposeConfig struct {
	Version  string
	Services map[string]Instance
	Volumes  map[string]map[string]string
}

type Options struct {
	NumZeros       int
	NumAlphas      int
	NumGroups      int
	LruSizeMB      int
	PersistData    bool
	EnterpriseMode bool
	TestPortRange  bool
}

var opts Options

const (
	zeroBasePort  int = 5080
	alphaBasePort int = 7080
)

func name(prefix string, idx int) string {
	return fmt.Sprintf("%s%d", prefix, idx)
}

func toExposedPort(i int) string {
	return fmt.Sprintf("%d:%d", i, i)
}

func binVolume() Volume {
	return Volume{
		Type:     "bind",
		Source:   "$GOPATH/bin",
		Target:   "/gobin",
		ReadOnly: true,
	}
}

func dataVolume() Volume {
	return Volume{
		Type:   "volume",
		Source: "data",
		Target: "/data",
	}
}

func getZero(idx int) Instance {
	prefix := "zero"
	svcName := name(prefix, idx)
	grpcPort := zeroBasePort + idx - 1
	httpPort := grpcPort + 1000

	var i Instance
	i.Image = "dgraph/dgraph:latest"
	i.ContainerName = svcName
	i.WorkingDir = fmt.Sprintf("/data/%s", i.ContainerName)
	if idx > 1 {
		i.DependsOn = append(i.DependsOn, name(prefix, idx-1))
	}
	i.Labels = map[string]string{"cluster": "test"}

	i.Ports = []string{
		toExposedPort(grpcPort),
		toExposedPort(httpPort),
	}

	i.Volumes = append(i.Volumes, binVolume())
	if opts.PersistData {
		i.Volumes = append(i.Volumes, dataVolume())
	}

	i.Command = fmt.Sprintf("/gobin/dgraph zero -o %d --idx=%d", idx-1, idx)
	i.Command += fmt.Sprintf(" --my=%s:%d", svcName, grpcPort)
	i.Command += fmt.Sprintf(" --replicas=%d", int(math.Ceil(float64(opts.NumAlphas)/float64(opts.NumGroups))))
	i.Command += " --logtostderr -v=2"
	if idx == 1 {
		i.Command += fmt.Sprintf(" --bindall")
	} else {
		i.Command += fmt.Sprintf(" --peer=%s:%d", name(prefix, 1), zeroBasePort)
	}

	return i
}
func getAlpha(idx int) Instance {
	baseOffset := 0
	if opts.TestPortRange {
		baseOffset += 100
	}

	prefix := "alpha"
	svcName := name(prefix, idx)
	itnlPort := alphaBasePort + baseOffset + idx - 1
	grpcPort := itnlPort + 1000
	httpPort := grpcPort + 1000

	var i Instance
	i.Image = "dgraph/dgraph:latest"
	i.ContainerName = svcName
	i.WorkingDir = fmt.Sprintf("/data/%s", i.ContainerName)
	if idx > 1 {
		i.DependsOn = append(i.DependsOn, name(prefix, idx-1))
	}
	i.Labels = map[string]string{"cluster": "test"}

	i.Ports = []string{
		toExposedPort(grpcPort),
		toExposedPort(httpPort),
	}

	i.Volumes = append(i.Volumes, binVolume())
	if opts.PersistData {
		i.Volumes = append(i.Volumes, dataVolume())
	}

	i.Command = fmt.Sprintf("/gobin/dgraph alpha -o %d", baseOffset+idx-1)
	i.Command += fmt.Sprintf(" --my=%s:%d", svcName, itnlPort)
	i.Command += fmt.Sprintf(" --lru_mb=%d", opts.LruSizeMB)
	i.Command += fmt.Sprintf(" --zero=zero1:%d", zeroBasePort)
	i.Command += " --logtostderr -v=2"
	i.Command += " --whitelist=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	if opts.EnterpriseMode {
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
	flag.IntVar(&opts.NumZeros, "num_zeros", 1,
		"number of zeros in dgraph cluster")
	flag.IntVar(&opts.NumAlphas, "num_alphas", 1,
		"number of alphas in dgraph cluster")
	flag.IntVar(&opts.NumGroups, "num_groups", 1,
		"number of groups in dgraph cluster")
	flag.IntVar(&opts.LruSizeMB, "lru_mb", 1024,
		"approximate size of LRU cache")
	flag.BoolVar(&opts.PersistData, "persist_data", false,
		"use a persistent data volume")
	flag.BoolVar(&opts.EnterpriseMode, "enterprise", false,
		"enable enterprise features in alphas")
	flag.BoolVar(&opts.TestPortRange, "test_ports", true,
		"use alpha ports expected by regression tests")
	flag.Parse()

	// Do some sanity checks.
	if opts.NumZeros < 1 || opts.NumZeros > 99 {
		fatal(fmt.Errorf("number of zeros must be 1-99"))
	}
	if opts.NumAlphas < 1 || opts.NumAlphas > 99 {
		fatal(fmt.Errorf("number of alphas must be 1-99"))
	}
	if opts.LruSizeMB < 1024 {
		fatal(fmt.Errorf("LRU cache size must be >= 1024 MB"))
	}

	services := make(map[string]Instance)

	for i := 1; i <= opts.NumZeros; i++ {
		instance := getZero(i)
		services[instance.ContainerName] = instance
	}

	for i := 1; i <= opts.NumAlphas; i++ {
		instance := getAlpha(i)
		services[instance.ContainerName] = instance
	}

	cfg := ComposeConfig{
		Version:  "3.5",
		Services: services,
	}
	if opts.PersistData {
		cfg.Volumes = make(map[string]map[string]string)
		cfg.Volumes["data"] = map[string]string{}
	}

	out, err := yaml.Marshal(cfg)
	x.Check(err)
	fmt.Printf("%s", out)
}
