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
	"io/ioutil"
	"os"
	"os/user"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"

	"github.com/dgraph-io/dgraph/x"
)

type StringMap map[string]string

type Volume struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool `yaml:"read_only"`
}

type Service struct {
	name          string // not exported
	Image         string
	ContainerName string    `yaml:"container_name"`
	Hostname      string    `yaml:",omitempty"`
	Pid           string    `yaml:",omitempty"`
	WorkingDir    string    `yaml:"working_dir,omitempty"`
	DependsOn     []string  `yaml:"depends_on,omitempty"`
	Labels        StringMap `yaml:",omitempty"`
	Environment   []string  `yaml:",omitempty"`
	Ports         []string  `yaml:",omitempty"`
	Volumes       []Volume  `yaml:",omitempty"`
	TempFS        []string  `yaml:",omitempty"`
	User          string    `yaml:",omitempty"`
	Command       string    `yaml:",omitempty"`
}

type ComposeConfig struct {
	Version  string
	Services map[string]Service
	Volumes  map[string]StringMap
}

type Options struct {
	NumZeros      int
	NumAlphas     int
	NumReplicas   int
	LruSizeMB     int
	Enterprise    bool
	AclSecret     string
	DataDir       string
	DataVol       bool
	TempFS        bool
	UserOwnership bool
	Jaeger        bool
	Metrics       bool
	PortOffset    int
	Verbosity     int
	OutFile       string
	LocalBin      bool
	WhiteList     bool
}

var opts Options

const (
	zeroBasePort  int = 5080 // HTTP=6080
	alphaBasePort int = 7080 // HTTP=8080, GRPC=9080
)

func name(prefix string, idx int) string {
	return fmt.Sprintf("%s%d", prefix, idx)
}

func toExposedPort(i int) string {
	return fmt.Sprintf("%d:%d", i, i)
}

func getOffset(idx int) int {
	if idx == 1 {
		return 0
	}
	return idx
}

func initService(basename string, idx, grpcPort int) Service {
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

	switch {
	case opts.DataVol == true:
		svc.Volumes = append(svc.Volumes, Volume{
			Type:   "volume",
			Source: "data",
			Target: "/data",
		})
	case opts.DataDir != "":
		svc.Volumes = append(svc.Volumes, Volume{
			Type:   "bind",
			Source: opts.DataDir,
			Target: "/data",
		})
	default:
		// no data volume
	}

	svc.Command = "dgraph"
	if opts.LocalBin {
		svc.Command = "/gobin/dgraph"
	}
	if opts.UserOwnership {
		user, err := user.Current()
		if err != nil {
			x.CheckfNoTrace(errors.Wrap(err, "unable to get current user"))
		}
		svc.User = fmt.Sprintf("${UID:-%s}", user.Uid)
		svc.WorkingDir = fmt.Sprintf("/working/%s", svc.name)
		svc.Command += fmt.Sprintf(" --cwd=/data/%s", svc.name)
	}
	svc.Command += " " + basename
	if opts.Jaeger {
		svc.Command += " --jaeger.collector=http://jaeger:14268"
	}

	return svc
}

func getZero(idx int) Service {
	basename := "zero"
	basePort := zeroBasePort + opts.PortOffset
	grpcPort := basePort + getOffset(idx)

	svc := initService(basename, idx, grpcPort)

	if opts.TempFS {
		svc.TempFS = append(svc.TempFS, fmt.Sprintf("/data/%s/zw", svc.name))
	}

	svc.Command += fmt.Sprintf(" -o %d --idx=%d", opts.PortOffset+getOffset(idx), idx)
	svc.Command += fmt.Sprintf(" --my=%s:%d", svc.name, grpcPort)
	if opts.NumAlphas > 1 {
		svc.Command += fmt.Sprintf(" --replicas=%d", opts.NumReplicas)
	}
	svc.Command += fmt.Sprintf(" --logtostderr -v=%d", opts.Verbosity)
	if idx == 1 {
		svc.Command += fmt.Sprintf(" --bindall")
	} else {
		svc.Command += fmt.Sprintf(" --peer=%s:%d", name(basename, 1), basePort)
	}

	return svc
}

func getAlpha(idx int) Service {
	basename := "alpha"
	internalPort := alphaBasePort + opts.PortOffset + getOffset(idx)
	grpcPort := internalPort + 1000

	svc := initService(basename, idx, grpcPort)

	if opts.TempFS {
		svc.TempFS = append(svc.TempFS, fmt.Sprintf("/data/%s/w", svc.name))
	}

	svc.Command += fmt.Sprintf(" -o %d", opts.PortOffset+getOffset(idx))
	svc.Command += fmt.Sprintf(" --my=%s:%d", svc.name, internalPort)
	svc.Command += fmt.Sprintf(" --lru_mb=%d", opts.LruSizeMB)
	svc.Command += fmt.Sprintf(" --zero=zero1:%d", zeroBasePort+opts.PortOffset)
	svc.Command += fmt.Sprintf(" --logtostderr -v=%d", opts.Verbosity)
	if opts.WhiteList {
		svc.Command += " --whitelist=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	}
	if opts.Enterprise {
		svc.Command += " --enterprise_features"
		if opts.AclSecret != "" {
			svc.Command += " --acl_secret_file=/secret/hmac --acl_access_ttl 3s --acl_cache_ttl 5s"
			svc.Volumes = append(svc.Volumes, Volume{
				Type:     "bind",
				Source:   opts.AclSecret,
				Target:   "/secret/hmac",
				ReadOnly: true,
			})
		}
	}

	return svc
}

func getJaeger() Service {
	svc := Service{
		Image:         "jaegertracing/all-in-one:latest",
		ContainerName: "jaeger",
		WorkingDir:    "/working/jaeger",
		Ports: []string{
			toExposedPort(14268),
			toExposedPort(16686),
		},
		Environment: []string{
			"SPAN_STORAGE_TYPE=badger",
		},
		Command: "--badger.ephemeral=false" +
			" --badger.directory-key /working/jaeger" +
			" --badger.directory-value /working/jaeger",
	}
	return svc
}

func addMetrics(cfg *ComposeConfig) {
	cfg.Volumes["prometheus-volume"] = StringMap{}
	cfg.Volumes["grafana-volume"] = StringMap{}

	cfg.Services["node-exporter"] = Service{
		Image:         "quay.io/prometheus/node-exporter",
		ContainerName: "node-exporter",
		Pid:           "host",
		WorkingDir:    "/working/jaeger",
		Volumes: []Volume{{
			Type:     "bind",
			Source:   "/",
			Target:   "/host",
			ReadOnly: true,
		}},
	}

	cfg.Services["prometheus"] = Service{
		Image:         "prom/prometheus",
		ContainerName: "prometheus",
		Hostname:      "prometheus",
		Ports: []string{
			toExposedPort(9090),
		},
		Volumes: []Volume{
			{
				Type:   "volume",
				Source: "prometheus-volume",
				Target: "/prometheus",
			},
			{
				Type:     "bind",
				Source:   "$GOPATH/src/github.com/dgraph-io/dgraph/compose/prometheus.yml",
				Target:   "/etc/prometheus/prometheus.yml",
				ReadOnly: true,
			},
		},
	}

	cfg.Services["grafana"] = Service{
		Image:         "grafana/grafana",
		ContainerName: "grafana",
		Hostname:      "grafana",
		Ports: []string{
			toExposedPort(3000),
		},
		Environment: []string{
			// Skip login
			"GF_AUTH_ANONYMOUS_ENABLED=true",
			"GF_AUTH_ANONYMOUS_ORG_ROLE=Admin",
		},
		Volumes: []Volume{{
			Type:   "volume",
			Source: "grafana-volume",
			Target: "/var/lib/grafana",
		}},
	}

	return
}

func warning(str string) {
	fmt.Fprintf(os.Stderr, "compose: %v\n", str)
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
		Example: "$ compose -z=3 -a=3",
		Run: func(cmd *cobra.Command, args []string) {
			// dummy to get "Usage:" template in Usage() output.
		},
	}

	cmd.PersistentFlags().IntVarP(&opts.NumZeros, "num_zeros", "z", 3,
		"number of zeros in dgraph cluster")
	cmd.PersistentFlags().IntVarP(&opts.NumAlphas, "num_alphas", "a", 3,
		"number of alphas in dgraph cluster")
	cmd.PersistentFlags().IntVarP(&opts.NumReplicas, "num_replicas", "r", 3,
		"number of alpha replicas in dgraph cluster")
	cmd.PersistentFlags().IntVar(&opts.LruSizeMB, "lru_mb", 1024,
		"approximate size of LRU cache")
	cmd.PersistentFlags().BoolVar(&opts.DataVol, "data_vol", false,
		"mount a docker volume as /data in containers")
	cmd.PersistentFlags().StringVarP(&opts.DataDir, "data_dir", "d", "",
		"mount a host directory as /data in containers")
	cmd.PersistentFlags().BoolVarP(&opts.Enterprise, "enterprise", "e", false,
		"enable enterprise features in alphas")
	cmd.PersistentFlags().StringVar(&opts.AclSecret, "acl_secret", "",
		"enable ACL feature with specified HMAC secret file")
	cmd.PersistentFlags().BoolVarP(&opts.UserOwnership, "user", "u", false,
		"run as the current user rather than root")
	cmd.PersistentFlags().BoolVar(&opts.TempFS, "tmpfs", false,
		"store w and zw directories on a tmpfs filesystem")
	cmd.PersistentFlags().BoolVarP(&opts.Jaeger, "jaeger", "j", false,
		"include jaeger service")
	cmd.PersistentFlags().BoolVarP(&opts.Metrics, "metrics", "m", false,
		"include metrics (prometheus, grafana) services")
	cmd.PersistentFlags().IntVarP(&opts.PortOffset, "port_offset", "o", 100,
		"port offset for alpha and, if not 100, zero as well")
	cmd.PersistentFlags().IntVarP(&opts.Verbosity, "verbosity", "v", 2,
		"glog verbosity level")
	cmd.PersistentFlags().StringVarP(&opts.OutFile, "out", "O",
		"./docker-compose.yml", "name of output file")
	cmd.PersistentFlags().BoolVarP(&opts.LocalBin, "local", "l", true,
		"use locally-compiled binary if true, otherwise use binary from docker container")
	cmd.PersistentFlags().BoolVarP(&opts.WhiteList, "whitelist", "w", false,
		"include a whitelist if true")

	err := cmd.ParseFlags(os.Args)
	if err != nil {
		if err == pflag.ErrHelp {
			_ = cmd.Usage()
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
	if opts.NumReplicas%2 == 0 {
		fatal(fmt.Errorf("number of replicas must be odd"))
	}
	if opts.LruSizeMB < 1024 {
		fatal(fmt.Errorf("LRU cache size must be >= 1024 MB"))
	}
	if opts.AclSecret != "" && !opts.Enterprise {
		warning("adding --enterprise because it is required by ACL feature")
		opts.Enterprise = true
	}
	if opts.DataVol && opts.DataDir != "" {
		fatal(fmt.Errorf("only one of --data_vol and --data_dir may be used at a time"))
	}
	if opts.UserOwnership && opts.DataDir == "" {
		fatal(fmt.Errorf("--user option requires --data_dir=<path>"))
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
		Volumes:  make(map[string]StringMap),
	}

	if opts.DataVol {
		cfg.Volumes["data"] = StringMap{}
	}

	if opts.Jaeger {
		services["jaeger"] = getJaeger()
	}

	if opts.Metrics {
		addMetrics(&cfg)
	}

	yml, err := yaml.Marshal(cfg)
	x.CheckfNoTrace(err)

	doc := fmt.Sprintf("# Auto-generated with: %v\n#\n", os.Args[:])
	if opts.UserOwnership {
		doc += fmt.Sprint("# NOTE: Env var UID must be exported by the shell\n#\n")
	}
	doc += fmt.Sprintf("%s", yml)
	if opts.OutFile == "-" {
		_, _ = fmt.Printf("%s", doc)
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "Writing file: %s\n", opts.OutFile)
		err = ioutil.WriteFile(opts.OutFile, []byte(doc), 0644)
		if err != nil {
			fatal(fmt.Errorf("unable to write file: %v", err))
		}
	}
}
