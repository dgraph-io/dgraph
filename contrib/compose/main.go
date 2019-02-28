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
	"fmt"
	"math"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"

	"github.com/dgraph-io/dgraph/x"
)

type Volume struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool `yaml:"read_only"`
}

type Service struct {
	name          string // not exported
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
	Services map[string]Service
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

func getService(basename string, idx, grpcPort int) Service {
	var svc Service

	svc.name = name(basename, idx)
	svc.Image = "dgraph/dgraph:latest"
	svc.ContainerName = svc.name
	svc.WorkingDir = fmt.Sprintf("/data/%s", svc.name)
	if idx > 1 {
		svc.DependsOn = append(svc.DependsOn, name(basename, idx-1))
	}
	svc.Labels = map[string]string{"cluster": "test"}

	svc.Ports = []string{
		toExposedPort(grpcPort),
		toExposedPort(grpcPort + 1000), // http port
	}

	svc.Volumes = append(svc.Volumes, Volume{
		Type:     "bind",
		Source:   "$GOPATH/bin",
		Target:   "/gobin",
		ReadOnly: true,
	})
	if opts.PersistData {
		svc.Volumes = append(svc.Volumes, Volume{
			Type:     "volume",
			Source:   "data",
			Target:   "/data",
			ReadOnly: false,
		})
	}

	return svc
}

func getZero(idx int) Service {
	basename := "zero"
	grpcPort := zeroBasePort + idx - 1

	svc := getService(basename, idx, grpcPort)

	svc.Command = fmt.Sprintf("/gobin/dgraph zero -o %d --idx=%d", idx-1, idx)
	svc.Command += fmt.Sprintf(" --my=%s:%d", svc.name, grpcPort)
	svc.Command += fmt.Sprintf(" --replicas=%d",
		int(math.Ceil(float64(opts.NumAlphas)/float64(opts.NumGroups))))
	svc.Command += " --logtostderr -v=2"
	if idx == 1 {
		svc.Command += fmt.Sprintf(" --bindall")
	} else {
		svc.Command += fmt.Sprintf(" --peer=%s:%d", name(basename, 1), zeroBasePort)
	}

	return svc
}

func getAlpha(idx int) Service {
	baseOffset := 0
	if opts.TestPortRange {
		baseOffset += 100
	}

	basename := "alpha"
	internalPort := alphaBasePort + baseOffset + idx - 1
	grpcPort := internalPort + 1000

	svc := getService(basename, idx, grpcPort)

	svc.Command = fmt.Sprintf("/gobin/dgraph alpha -o %d", baseOffset+idx-1)
	svc.Command += fmt.Sprintf(" --my=%s:%d", svc.name, internalPort)
	svc.Command += fmt.Sprintf(" --lru_mb=%d", opts.LruSizeMB)
	svc.Command += fmt.Sprintf(" --zero=zero1:%d", zeroBasePort)
	svc.Command += " --logtostderr -v=2"
	svc.Command += " --whitelist=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	if opts.EnterpriseMode {
		svc.Command += " --enterprise_features"
	}

	return svc
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "compose: %v\n", err)
	os.Exit(1)
}

func main() {
	var cmd = &cobra.Command{
		Use:     "compose",
		Short:   "docker-compose config file generator for dgraph",
		Long:    "Dynamically generate a docker-compose.yml file for running a dgraph cluster.",
		Example: "$ compose --num_zeros=3 --num_alphas=3 | docker-compose -f- up",
		Run: func(cmd *cobra.Command, args []string) {
			// dummy to get "Usage:" template in Usage() output.
		},
	}

	cmd.PersistentFlags().IntVarP(&opts.NumZeros, "num_zeros", "z", 1,
		"number of zeros in dgraph cluster")
	cmd.PersistentFlags().IntVarP(&opts.NumAlphas, "num_alphas", "a", 1,
		"number of alphas in dgraph cluster")
	cmd.PersistentFlags().IntVarP(&opts.NumGroups, "num_groups", "g", 1,
		"number of groups in dgraph cluster")
	cmd.PersistentFlags().IntVar(&opts.LruSizeMB, "lru_mb", 1024,
		"approximate size of LRU cache")
	cmd.PersistentFlags().BoolVarP(&opts.PersistData, "persist_data", "p", false,
		"use a persistent data volume")
	cmd.PersistentFlags().BoolVarP(&opts.EnterpriseMode, "enterprise", "e", false,
		"enable enterprise features in alphas")
	cmd.PersistentFlags().BoolVar(&opts.TestPortRange, "test_ports", true,
		"use alpha ports expected by regression tests")

	err := cmd.ParseFlags(os.Args)
	if err != nil {
		if err == pflag.ErrHelp {
			cmd.Usage()
			os.Exit(0)
		}
		fatal(err)
	}

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

	services := make(map[string]Service)

	for i := 1; i <= opts.NumZeros; i++ {
		svc := getZero(i)
		services[svc.name] = svc
	}

	for i := 1; i <= opts.NumAlphas; i++ {
		svc := getAlpha(i)
		services[svc.name] = svc
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
	fmt.Printf("# Auto-generated with: %v\n#\n", os.Args[:])
	fmt.Printf("%s", out)
}
