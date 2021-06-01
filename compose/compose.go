/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *	 http://www.apache.org/licenses/LICENSE-2.0
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
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"

	sv "github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	yaml "gopkg.in/yaml.v2"

	"github.com/dgraph-io/dgraph/x"
)

type stringMap map[string]string

type volume struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool `yaml:"read_only"`
}

type deploy struct {
	Resources res `yaml:",omitempty"`
}

type res struct {
	Limits limit `yaml:",omitempty"`
}

type limit struct {
	Memory string `yaml:",omitempty"`
}

type service struct {
	name          string // not exported
	Image         string
	ContainerName string    `yaml:"container_name,omitempty"`
	Hostname      string    `yaml:",omitempty"`
	Pid           string    `yaml:",omitempty"`
	WorkingDir    string    `yaml:"working_dir,omitempty"`
	DependsOn     []string  `yaml:"depends_on,omitempty"`
	Labels        stringMap `yaml:",omitempty"`
	EnvFile       []string  `yaml:"env_file,omitempty"`
	Environment   []string  `yaml:",omitempty"`
	Ports         []string  `yaml:",omitempty"`
	Volumes       []volume  `yaml:",omitempty"`
	TmpFS         []string  `yaml:",omitempty"`
	User          string    `yaml:",omitempty"`
	Command       string    `yaml:",omitempty"`
	Deploy        deploy    `yaml:",omitempty"`
}

type composeConfig struct {
	Version  string
	Services map[string]service
	Volumes  map[string]stringMap
}

type options struct {
	NumZeros       int
	NumAlphas      int
	NumReplicas    int
	NumLearners    int
	Acl            bool
	AclSecret      string
	DataDir        string
	PDir           string
	DataVol        bool
	TmpFS          bool
	UserOwnership  bool
	Jaeger         bool
	Metrics        bool
	PortOffset     int
	Verbosity      int
	Vmodule        string
	OutFile        string
	LocalBin       bool
	Image          string
	Tag            string
	WhiteList      bool
	MemLimit       string
	ExposePorts    bool
	Encryption     bool
	SnapshotAfter  string
	ContainerNames bool
	AlphaVolumes   []string
	ZeroVolumes    []string
	AlphaEnvFile   []string
	ZeroEnvFile    []string
	Minio          bool
	MinioDataDir   string
	MinioPort      uint16
	MinioEnvFile   []string
	Hostname       string
	Cdc            bool
	CdcConsumer    bool

	// Alpha Configurations
	CustomAlphaOptions []string

	// Container Alias
	ContainerPrefix string

	// Extra flags
	AlphaFlags string
	ZeroFlags  string
}

var opts options

const (
	zeroBasePort  int = 5080 // HTTP=6080
	alphaBasePort int = 7080 // HTTP=8080, GRPC=9080
)

func name(prefix string, idx int) string {
	return fmt.Sprintf("%s%d", prefix, idx)
}

func containerName(s string) string {
	if opts.ContainerNames {
		return s
	}
	return ""
}

func toPort(i int) string {
	if opts.ExposePorts {
		return fmt.Sprintf("%d:%d", i, i)
	}
	return fmt.Sprintf("%d", i)
}

func getOffset(idx int) int {
	if !opts.ExposePorts {
		return 0
	}
	if idx == 1 {
		return 0
	}
	return idx
}

func getHost(host string) string {
	if opts.Hostname != "" {
		return opts.Hostname
	}
	return host
}

func initService(basename string, idx, grpcPort int) service {
	var svc service
	containerPrefix := basename
	if opts.ContainerPrefix != "" {
		containerPrefix = opts.ContainerPrefix + "_" + basename
	}
	svc.name = name(containerPrefix, idx)
	svc.Image = opts.Image + ":" + opts.Tag
	svc.ContainerName = containerName(svc.name)
	svc.WorkingDir = fmt.Sprintf("/data/%s", svc.name)
	if idx > 1 {
		svc.DependsOn = append(svc.DependsOn, name(basename, idx-1))
	}
	svc.Labels = map[string]string{"cluster": "test"}

	svc.Ports = []string{
		toPort(grpcPort),
		toPort(grpcPort + 1000), // http port
	}

	// If hostname is specified then expose the internal grpc port (7080) of alpha.
	if basename == "alpha" && opts.Hostname != "" {
		svc.Ports = append(svc.Ports, toPort(grpcPort-1000))
	}
	if opts.LocalBin {
		svc.Volumes = append(svc.Volumes, volume{
			Type:     "bind",
			Source:   "$GOPATH/bin",
			Target:   "/gobin",
			ReadOnly: true,
		})
	}

	switch {
	case opts.DataVol:
		svc.Volumes = append(svc.Volumes, volume{
			Type:   "volume",
			Source: "data",
			Target: "/data",
		})
	case opts.DataDir != "":
		svc.Volumes = append(svc.Volumes, volume{
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
		svc.Command += ` --trace "jaeger=http://jaeger:14268;"`
	}
	return svc
}

func getZero(idx int, raft string) service {
	basename := "zero"
	basePort := zeroBasePort + opts.PortOffset
	grpcPort := basePort + getOffset(idx)

	svc := initService(basename, idx, grpcPort)

	if opts.TmpFS {
		svc.TmpFS = append(svc.TmpFS, fmt.Sprintf("/data/%s/zw", svc.name))
	}

	offset := getOffset(idx)
	if (opts.PortOffset + offset) != 0 {
		svc.Command += fmt.Sprintf(" -o %d", opts.PortOffset+offset)
	}
	svc.Command += fmt.Sprintf(" --raft='%s'", raft)
	svc.Command += fmt.Sprintf(" --my=%s:%d", getHost(svc.name), grpcPort)
	if opts.NumAlphas > 1 {
		svc.Command += fmt.Sprintf(" --replicas=%d", opts.NumReplicas)
	}
	svc.Command += fmt.Sprintf(" --logtostderr -v=%d", opts.Verbosity)
	if opts.Vmodule != "" {
		svc.Command += fmt.Sprintf(" --vmodule=%s", opts.Vmodule)
	}
	if idx == 1 {
		svc.Command += fmt.Sprintf(" --bindall")
	} else {
		peerHost := name(basename, 1)
		svc.Command += fmt.Sprintf(" --peer=%s:%d", getHost(peerHost), basePort)
	}
	if len(opts.MemLimit) > 0 {
		svc.Deploy.Resources = res{
			Limits: limit{Memory: opts.MemLimit},
		}
	}
	if opts.ZeroFlags != "" {
		svc.Command += " " + opts.ZeroFlags
	}

	if len(opts.ZeroVolumes) > 0 {
		for _, vol := range opts.ZeroVolumes {
			svc.Volumes = append(svc.Volumes, getVolume(vol))
		}
	}
	svc.EnvFile = opts.ZeroEnvFile

	return svc
}

func getAlpha(idx int, raft string, customFlags string) service {
	basename := "alpha"
	internalPort := alphaBasePort + opts.PortOffset + getOffset(idx)
	grpcPort := internalPort + 1000
	svc := initService(basename, idx, grpcPort)

	if opts.TmpFS {
		svc.TmpFS = append(svc.TmpFS, fmt.Sprintf("/data/%s/w", svc.name))
	}

	isMultiZeros := true
	var isInvalidVersion, err = semverCompare("< 1.2.3 || 20.03.0", opts.Tag)
	if err != nil || isInvalidVersion {
		if opts.Tag != "latest" {
			isMultiZeros = false
		}
	}

	maxZeros := 1
	if isMultiZeros {
		maxZeros = opts.NumZeros
	}

	zeroName := "zero"
	if opts.ContainerPrefix != "" {
		zeroName = opts.ContainerPrefix + "_" + zeroName
	}

	zeroHostAddr := fmt.Sprintf("%s:%d", getHost(zeroName+"1"), zeroBasePort+opts.PortOffset)
	zeros := []string{zeroHostAddr}
	for i := 2; i <= maxZeros; i++ {
		port := zeroBasePort + opts.PortOffset + getOffset(i)
		zeroHost := fmt.Sprintf("%s%d", zeroName, i)
		zeroHostAddr = fmt.Sprintf("%s:%d", getHost(zeroHost), port)
		zeros = append(zeros, zeroHostAddr)
	}

	zerosOpt := strings.Join(zeros, ",")

	offset := getOffset(idx)
	if (opts.PortOffset + offset) != 0 {
		svc.Command += fmt.Sprintf(" -o %d", opts.PortOffset+offset)
	}
	svc.Command += fmt.Sprintf(" --my=%s:%d", getHost(svc.name), internalPort)
	svc.Command += fmt.Sprintf(" --zero=%s", zerosOpt)
	svc.Command += fmt.Sprintf(" --logtostderr -v=%d", opts.Verbosity)
	svc.Command += " --expose_trace=true"

	if opts.SnapshotAfter != "" {
		raft = fmt.Sprintf("%s; %s", raft, opts.SnapshotAfter)
	}
	svc.Command += fmt.Sprintf(` --raft "%s"`, raft)

	// Don't assign idx, let it auto-assign.
	// svc.Command += fmt.Sprintf(" --raft='idx=%d'", idx)
	if opts.Vmodule != "" {
		svc.Command += fmt.Sprintf(" --vmodule=%s", opts.Vmodule)
	}
	if opts.WhiteList {
		svc.Command += ` --security "whitelist=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.0.0.0/8;"`
	}
	if opts.Acl {
		svc.Command += ` --acl "secret-file=/secret/hmac;"`
		svc.Volumes = append(svc.Volumes, volume{
			Type:     "bind",
			Source:   "./acl-secret",
			Target:   "/secret/hmac",
			ReadOnly: true,
		})
	}
	if opts.AclSecret != "" {
		svc.Command += ` --acl "secret-file=/secret/hmac;"`
		svc.Volumes = append(svc.Volumes, volume{
			Type:     "bind",
			Source:   opts.AclSecret,
			Target:   "/secret/hmac",
			ReadOnly: true,
		})
	}
	if len(opts.MemLimit) > 0 {
		svc.Deploy.Resources = res{
			Limits: limit{Memory: opts.MemLimit},
		}
	}
	if opts.Encryption {
		svc.Command += ` --encryption "key-file=/secret/enc_key;"`
		svc.Volumes = append(svc.Volumes, volume{
			Type:     "bind",
			Source:   "./enc-secret",
			Target:   "/secret/enc_key",
			ReadOnly: true,
		})
	}
	if opts.Cdc {
		svc.Command += " --cdc='kafka=kafka:9092'"
	}
	if len(opts.AlphaVolumes) > 0 {
		for _, vol := range opts.AlphaVolumes {
			svc.Volumes = append(svc.Volumes, getVolume(vol))
		}
	}
	svc.EnvFile = opts.AlphaEnvFile
	if opts.AlphaFlags != "" {
		svc.Command += " " + opts.AlphaFlags
	}

	if customFlags != "" {
		svc.Command += " " + customFlags
	}

	return svc
}

func getVolume(vol string) volume {
	s := strings.Split(vol, ":")
	srcDir := s[0]
	dstDir := s[1]
	readOnly := len(s) > 2 && s[2] == "ro"
	volType := "volume"
	if isBindMount(srcDir) {
		volType = "bind"
	}
	return volume{
		Type:     volType,
		Source:   srcDir,
		Target:   dstDir,
		ReadOnly: readOnly,
	}

}

func getJaeger() service {
	svc := service{
		Image:         "jaegertracing/all-in-one:1.18",
		ContainerName: containerName("jaeger"),
		WorkingDir:    "/working/jaeger",
		Ports: []string{
			toPort(14268),
			toPort(16686),
		},
		Environment: []string{
			"SPAN_STORAGE_TYPE=memory",
			// "SPAN_STORAGE_TYPE=badger",
			// Note: Badger doesn't quite work as well in Jaeger. The integration isn't well
			// written.
		},
		// Command: "--badger.ephemeral=false" +
		// 	" --badger.directory-key /working/jaeger" +
		// 	" --badger.directory-value /working/jaeger",
	}
	return svc
}

func getMinio(minioDataDir string) service {
	svc := service{
		Image:         "minio/minio:RELEASE.2020-11-13T20-10-18Z",
		ContainerName: containerName("minio1"),
		Ports: []string{
			toPort(int(opts.MinioPort)),
		},
		EnvFile: opts.MinioEnvFile,
		Command: "minio server /data/minio --address :" +
			strconv.FormatUint(uint64(opts.MinioPort), 10),
	}
	if minioDataDir != "" {
		svc.Volumes = append(svc.Volumes, volume{
			Type:   "bind",
			Source: minioDataDir,
			Target: "/data/minio",
		})
	}
	return svc
}

func addMetrics(cfg *composeConfig) {
	cfg.Volumes["prometheus-volume"] = stringMap{}
	cfg.Volumes["grafana-volume"] = stringMap{}

	cfg.Services["node-exporter"] = service{
		Image:         "quay.io/prometheus/node-exporter:v1.0.1",
		ContainerName: containerName("node-exporter"),
		Pid:           "host",
		WorkingDir:    "/working/jaeger",
		Volumes: []volume{{
			Type:     "bind",
			Source:   "/",
			Target:   "/host",
			ReadOnly: true,
		}},
	}

	cfg.Services["prometheus"] = service{
		Image:         "prom/prometheus:v2.20.1",
		ContainerName: containerName("prometheus"),
		Hostname:      "prometheus",
		Ports: []string{
			toPort(9090),
		},
		Volumes: []volume{
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

	cfg.Services["grafana"] = service{
		Image:         "grafana/grafana:7.1.2",
		ContainerName: containerName("grafana"),
		Hostname:      "grafana",
		Ports: []string{
			toPort(3000),
		},
		Environment: []string{
			// Skip login
			"GF_AUTH_ANONYMOUS_ENABLED=true",
			"GF_AUTH_ANONYMOUS_ORG_ROLE=Admin",
		},
		Volumes: []volume{{
			Type:   "volume",
			Source: "grafana-volume",
			Target: "/var/lib/grafana",
		}},
	}
}

func addCdc(cfg *composeConfig) {
	cfg.Services["zookeeper"] = service{
		Image:         "bitnami/zookeeper:3.7.0",
		ContainerName: containerName("zookeeper"),
		Environment: []string{
			"ALLOW_ANONYMOUS_LOGIN=yes",
		},
	}
	cfg.Services["kafka"] = service{
		Image:         "bitnami/kafka:2.7.0",
		ContainerName: containerName("kafka"),
		Environment: []string{
			"ALLOW_PLAINTEXT_LISTENER=yes",
			"KAFKA_BROKER_ID=1",
			"KAFKA_CFG_ZOOKEEPER_CONNECT=zookeeper:2181",
		},
	}
	if opts.CdcConsumer {
		cfg.Services["kafka-consumer"] = service{
			Image:         "bitnami/kafka:2.7.0",
			ContainerName: containerName("kafka-consumer"),
			Command:       "kafka-console-consumer.sh --bootstrap-server kafka:9092 --topic dgraph-cdc",
		}
	}
}

func semverCompare(constraint, version string) (bool, error) {
	c, err := sv.NewConstraint(constraint)
	if err != nil {
		return false, err
	}

	v, err := sv.NewVersion(version)
	if err != nil {
		return false, err
	}

	return c.Check(v), nil
}

func isBindMount(vol string) bool {
	return strings.HasPrefix(vol, ".") || strings.HasPrefix(vol, "/")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "compose: %v\n", err)
	os.Exit(1)
}

func makeDir(path string) error {
	var err1 error
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		if errs := os.MkdirAll(path, 0755); errs != nil {
			err1 = errors.Wrapf(err, "Couldn't create directory %v.", path)
		}
	} else if err != nil {
		err1 = errors.Wrapf(err, "Something went wrong while checking if directory %v still exists.",
			path)
	}
	return err1
}

func copyFile(src, dst string) error {
	var err, err1 error
	var srcfd *os.File
	var dstfd *os.File
	var srcInfo os.FileInfo

	if srcfd, err = os.Open(src); err != nil {
		err1 = errors.Wrapf(err, "Error in opening source file %v.", src)
		return err1
	}
	defer srcfd.Close()

	if dstfd, err = os.Create(dst); err != nil {
		err1 = errors.Wrapf(err, "Error in creating destination file %v.", dst)
		return err1
	}
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		err1 = errors.Wrapf(err, "Error in copying source file %v to destination file %v.",
			src, dst)
		return err1
	}
	if srcInfo, err = os.Stat(src); err != nil {
		err1 = errors.Wrapf(err, "Error in doing stat of source file %v.", src)
		return err1
	}
	return os.Chmod(dst, srcInfo.Mode())
}

func copyDir(src string, dst string) error {
	var err, err1 error
	var fds []os.FileInfo
	var srcInfo os.FileInfo

	if srcInfo, err = os.Stat(src); err != nil {
		err1 = errors.Wrapf(err, "Error in doing stat of source dir %v.", src)
		return err1
	}

	if err = os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		err1 = errors.Wrapf(err, "Error in making dir %v.", dst)
		return err1
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		err1 = errors.Wrapf(err, "Error in reading source dir %v.", src)
		return err1
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = copyDir(srcfp, dstfp); err != nil {
				err1 = errors.Wrapf(err, "Could not copy dir %v to %v.", srcfp, dstfp)
				return err1
			}
		} else {
			if err = copyFile(srcfp, dstfp); err != nil {
				err1 = errors.Wrapf(err, "Could not copy file %v to %v.", srcfp, dstfp)
				return err1
			}
		}
	}
	return nil
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
		"number of zeros in Dgraph cluster")
	cmd.PersistentFlags().IntVarP(&opts.NumAlphas, "num_alphas", "a", 3,
		"number of alphas in Dgraph cluster")
	cmd.PersistentFlags().IntVarP(&opts.NumReplicas, "num_replicas", "r", 3,
		"number of alpha replicas in Dgraph cluster")
	cmd.PersistentFlags().IntVarP(&opts.NumLearners, "num_learners", "n", 0,
		"number of learner replicas in Dgraph cluster")
	cmd.PersistentFlags().BoolVar(&opts.DataVol, "data_vol", false,
		"mount a docker volume as /data in containers")
	cmd.PersistentFlags().StringVarP(&opts.DataDir, "data_dir", "d", "",
		"mount a host directory as /data in containers")
	cmd.PersistentFlags().StringVarP(&opts.PDir, "postings", "p", "",
		"launch cluster with local path of p directory, data_vol must be set to true and a=r."+
			"\nFor new cluster to pick postings, you might have to move uids and timestamp..."+
			"\ncurl \"http://localhost:<zeroPort>/assign?what=timestamps&num=1000000\""+
			"\ncurl \"http://localhost:<zeroPort>/assign?what=uids&num=1000000\"")

	cmd.PersistentFlags().BoolVar(&opts.Acl, "acl", false, "Create ACL secret file and enable ACLs")
	cmd.PersistentFlags().StringVar(&opts.AclSecret, "acl_secret", "",
		"enable ACL feature with specified HMAC secret file")
	cmd.PersistentFlags().BoolVarP(&opts.UserOwnership, "user", "u", false,
		"run as the current user rather than root")
	cmd.PersistentFlags().BoolVar(&opts.TmpFS, "tmpfs", false,
		"store w and zw directories on a tmpfs filesystem")
	cmd.PersistentFlags().BoolVarP(&opts.Jaeger, "jaeger", "j", false,
		"include jaeger service")
	cmd.PersistentFlags().BoolVarP(&opts.Metrics, "metrics", "m", false,
		"include metrics (prometheus, grafana) services")
	cmd.PersistentFlags().IntVarP(&opts.PortOffset, "port_offset", "o", 0,
		"port offset for alpha and zero")
	cmd.PersistentFlags().IntVarP(&opts.Verbosity, "verbosity", "v", 2,
		"glog verbosity level")
	cmd.PersistentFlags().StringVarP(&opts.OutFile, "out", "O",
		"./docker-compose.yml", "name of output file")
	cmd.PersistentFlags().BoolVarP(&opts.LocalBin, "local", "l", true,
		"use locally-compiled binary if true, otherwise use binary from docker container")
	cmd.PersistentFlags().StringVar(&opts.Image, "image", "dgraph/dgraph",
		"Docker image for alphas and zeros.")
	cmd.PersistentFlags().StringVarP(&opts.Tag, "tag", "t", "latest",
		"Docker tag for the --image image. Requires -l=false to use binary from docker container.")
	cmd.PersistentFlags().BoolVarP(&opts.WhiteList, "whitelist", "w", true,
		"include a whitelist if true")
	cmd.PersistentFlags().StringVarP(&opts.MemLimit, "mem", "", "32G",
		"Limit memory provided to the docker containers, for example 8G.")
	cmd.PersistentFlags().BoolVar(&opts.ExposePorts, "expose_ports", true,
		"expose host:container ports for each service")
	cmd.PersistentFlags().StringVar(&opts.Vmodule, "vmodule", "",
		"comma-separated list of pattern=N settings for file-filtered logging")
	cmd.PersistentFlags().BoolVar(&opts.Encryption, "encryption", false,
		"enable encryption-at-rest feature.")
	cmd.PersistentFlags().StringVar(&opts.SnapshotAfter, "snapshot_after", "",
		"create a new Raft snapshot after this many number of Raft entries.")
	cmd.PersistentFlags().StringVar(&opts.AlphaFlags, "extra_alpha_flags", "",
		"extra flags for alphas.")
	cmd.PersistentFlags().StringVar(&opts.ZeroFlags, "extra_zero_flags", "",
		"extra flags for zeros.")
	cmd.PersistentFlags().BoolVar(&opts.ContainerNames, "names", true,
		"set container names in docker compose.")
	cmd.PersistentFlags().StringArrayVar(&opts.AlphaVolumes, "alpha_volume", nil,
		"alpha volume mounts, following srcdir:dstdir[:ro]")
	cmd.PersistentFlags().StringArrayVar(&opts.ZeroVolumes, "zero_volume", nil,
		"zero volume mounts, following srcdir:dstdir[:ro]")
	cmd.PersistentFlags().StringArrayVar(&opts.AlphaEnvFile, "alpha_env_file", nil,
		"env_file for alpha")
	cmd.PersistentFlags().StringArrayVar(&opts.ZeroEnvFile, "zero_env_file", nil,
		"env_file for zero")
	cmd.PersistentFlags().BoolVar(&opts.Minio, "minio", false,
		"include minio service")
	cmd.PersistentFlags().StringVar(&opts.MinioDataDir, "minio_data_dir", "",
		"default minio data directory")
	cmd.PersistentFlags().Uint16Var(&opts.MinioPort, "minio_port", 9001,
		"minio service port")
	cmd.PersistentFlags().StringArrayVar(&opts.MinioEnvFile, "minio_env_file", nil,
		"minio service env_file")
	cmd.PersistentFlags().StringVar(&opts.ContainerPrefix, "prefix", "",
		"prefix for the container name")
	cmd.PersistentFlags().StringArrayVar(&opts.CustomAlphaOptions, "custom_alpha_options", nil,
		"Custom alpha flags for specific alphas,"+
			" following {\"1:custom_flags\", \"2:custom_flags\"}, eg: {\"2: -p <bulk_path>\"")
	cmd.PersistentFlags().StringVar(&opts.Hostname, "hostname", "",
		"hostname for the alpha and zero servers")
	cmd.PersistentFlags().BoolVar(&opts.Cdc, "cdc", false,
		"run Kafka and push CDC data to it")
	cmd.PersistentFlags().BoolVar(&opts.CdcConsumer, "cdc_consumer", false,
		"run Kafka consumer that prints out CDC events")
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
		fatal(errors.Errorf("number of zeros must be 1-99"))
	}
	if opts.NumAlphas < 0 || opts.NumAlphas > 99 {
		fatal(errors.Errorf("number of alphas must be 0-99"))
	}
	if opts.NumReplicas%2 == 0 {
		fatal(errors.Errorf("number of replicas must be odd"))
	}
	if opts.DataVol && opts.DataDir != "" {
		fatal(errors.Errorf("only one of --data_vol and --data_dir may be used at a time"))
	}
	if opts.UserOwnership && opts.DataDir == "" {
		fatal(errors.Errorf("--user option requires --data_dir=<path>"))
	}
	if cmd.Flags().Changed("cdc-consumer") && !opts.Cdc {
		fatal(errors.Errorf("--cdc_consumer requires --cdc"))
	}
	if opts.PDir != "" && opts.DataDir == "" {
		fatal(errors.Errorf("--postings option requires --data_dir"))
	}
	if opts.PDir != "" && opts.NumAlphas > opts.NumReplicas {
		fatal(errors.Errorf("--postings requires --num_replicas >= --num_alphas"))
	}

	services := make(map[string]service)

	for i := 1; i <= opts.NumZeros; i++ {
		svc := getZero(i, fmt.Sprintf("idx=%d", i))
		services[svc.name] = svc
	}

	// Alpha Customization
	customAlphas := make(map[int]string)
	for _, flag := range opts.CustomAlphaOptions {
		splits := strings.SplitN(flag, ":", 2)
		if len(splits) != 2 {
			fatal(errors.Errorf("custom_alpha_options, requires string in index:options format."))
		}
		idx, err := strconv.Atoi(splits[0])
		if err != nil {
			fatal(errors.Errorf(" custom_alpha_options, captured erros while parsing index value %v", err))
		}
		customAlphas[idx] = splits[1]
	}

	for i := 1; i <= opts.NumAlphas; i++ {
		gid := int(math.Ceil(float64(i) / float64(opts.NumReplicas)))
		rs := fmt.Sprintf("idx=%d; group=%d", i, gid)
		svc := getAlpha(i, rs, customAlphas[i])
		// Don't make Alphas depend on each other.
		svc.DependsOn = nil
		services[svc.name] = svc
	}

	numGroups := opts.NumAlphas / opts.NumReplicas
	lidx := opts.NumZeros
	for i := 1; i <= opts.NumLearners; i++ {
		lidx++
		rs := fmt.Sprintf("idx=%d; learner=true", lidx)
		svc := getZero(lidx, rs)
		services[svc.name] = svc
	}
	lidx = opts.NumAlphas
	for gid := 1; gid <= numGroups; gid++ {
		for i := 1; i <= opts.NumLearners; i++ {
			lidx++
			rs := fmt.Sprintf("idx=%d; group=%d; learner=true", lidx, gid)
			svc := getAlpha(lidx, rs, customAlphas[i])
			services[svc.name] = svc
		}
	}

	cfg := composeConfig{
		Version:  "3.5",
		Services: services,
		Volumes:  make(map[string]stringMap),
	}

	if len(opts.AlphaVolumes) > 0 {
		for _, vol := range opts.AlphaVolumes {
			s := strings.Split(vol, ":")
			srcDir := s[0]
			if !isBindMount(srcDir) {
				cfg.Volumes[srcDir] = stringMap{}
			}
		}
	}
	if len(opts.ZeroVolumes) > 0 {
		for _, vol := range opts.ZeroVolumes {
			s := strings.Split(vol, ":")
			srcDir := s[0]
			if !isBindMount(srcDir) {
				cfg.Volumes[srcDir] = stringMap{}
			}
		}
	}

	if opts.PDir != "" {
		if _, err := os.Stat(opts.DataDir); !os.IsNotExist(err) {
			fatal(errors.Errorf("Directory %v already exists.", opts.DataDir))
		}

		n := 1
		for n <= opts.NumAlphas {
			newDir := opts.DataDir + "/alpha" + strconv.Itoa(n) + "/p"
			err := makeDir(newDir)
			if err != nil {
				fatal(errors.Errorf("Couldn't create directory %v. Error: %v.", newDir, err))
			}
			err = copyDir(opts.PDir, newDir)
			if err != nil {
				fatal(errors.Errorf("Couldn't copy directory from %v to %v. Error: %v.",
					opts.PDir, newDir, err))
			}
			n++
		}
	}

	if opts.DataVol {
		cfg.Volumes["data"] = stringMap{}
	}

	if opts.Jaeger {
		services["jaeger"] = getJaeger()
	}

	if opts.Metrics {
		addMetrics(&cfg)
	}

	if opts.Minio {
		services["minio1"] = getMinio(opts.MinioDataDir)
	}

	if opts.Acl {
		err = ioutil.WriteFile("acl-secret", []byte("12345678901234567890123456789012"), 0644)
		x.Check2(fmt.Fprintf(os.Stdout, "Writing file: %s\n", "acl-secret"))
		if err != nil {
			fatal(errors.Errorf("unable to write file: %v", err))
		}
	}
	if opts.Encryption {
		err = ioutil.WriteFile("enc-secret", []byte("12345678901234567890123456789012"), 0644)
		x.Check2(fmt.Fprintf(os.Stdout, "Writing file: %s\n", "enc-secret"))
		if err != nil {
			fatal(errors.Errorf("unable to write file: %v", err))
		}
	}

	if opts.Cdc {
		addCdc(&cfg)
	}

	yml, err := yaml.Marshal(cfg)
	x.CheckfNoTrace(err)

	doc := fmt.Sprintf("# Auto-generated with: %v\n#\n", os.Args)
	if opts.UserOwnership {
		doc += fmt.Sprint("# NOTE: Env var UID must be exported by the shell\n#\n")
	}
	doc += fmt.Sprintf("%s", yml)
	if opts.OutFile == "-" {
		x.Check2(fmt.Printf("%s", doc))
	} else {
		fmt.Printf("Options: %+v\n", opts)
		fmt.Printf("Writing file: %s\n", opts.OutFile)
		err = ioutil.WriteFile(opts.OutFile, []byte(doc), 0644)
		if err != nil {
			fatal(errors.Errorf("unable to write file: %v", err))
		}
	}

	if opts.PDir != "" {
		fmt.Printf("For new cluster to pick \"postings\", you might have to move uids and timestamp..." +
			"\n\tcurl \"http://localhost:<zeroPort>/assign?what=timestamps&num=1000000\"" +
			"\n\tcurl \"http://localhost:<zeroPort>/assign?what=uids&num=1000000\"\n")
	}
}
