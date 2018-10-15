/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var opt struct {
	zero   string
	tmpDir string
	target string
	source string
}

var tlsConf x.TLSHelperConfig

var Backup x.SubCommand

func init() {
	Backup.Cmd = &cobra.Command{
		Use:   "backup",
		Short: "Run Dgraph backup",
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Backup.Conf).Stop()
			if err := run(); err != nil {
				x.Printf("Backup: %s\n", err)
				os.Exit(1)
			}
		},
	}
	Backup.EnvPrefix = "DGRAPH_BACKUP"

	flag := Backup.Cmd.Flags()
	flag.StringVarP(&opt.zero, "zero", "z", "127.0.0.1:5080", "Dgraphzero gRPC server address")
	flag.StringVar(&opt.tmpDir, "tmpdir", "", "Directory to store temporary files")
	flag.StringVarP(&opt.target, "target", "t", "", "URI path target to store the backups")
	flag.StringVarP(&opt.source, "source", "s", "", "URI path source containing full backups")
	x.RegisterTLSFlags(flag)
	Backup.Cmd.MarkFlagRequired("target")
}

func run() error {
	x.LoadTLSConfig(&tlsConf, Backup.Conf)
	x.Printf("%#v\n", opt)

	go http.ListenAndServe("localhost:6060", nil)

	b, err := setup()
	if err != nil {
		return err
	}
	fmt.Printf("Backup: saving to: %s ...\n", b.dst)
	if err := b.process(); err != nil {
		return err
	}
	fmt.Printf("Backup: time elapsed: %s\n", time.Since(b.start))

	return nil
}

func setup() (*backup, error) {
	h, err := findHandler(opt.target)
	if err != nil {
		return nil, x.Errorf("Backup: failed to find target handler: %s", err)
	}

	connzero, err := setupConnection(opt.zero, true)
	if err != nil {
		return nil, x.Errorf("Backup: unable to connect to zero at %q: %s", opt.zero, err)
	}

	start := time.Now()
	b := &backup{
		start:    start,
		zeroconn: connzero,
		target:   h,
		dst:      dgraphBackupFullPrefix + start.Format(time.RFC3339) + dgraphBackupSuffix,
	}

	if opt.source != "" {
		b.source, err = findHandler(opt.source)
		if err != nil {
			return nil, x.Errorf("Backup: failed to find a source handler: %s", err)
		}
	}

	b.incremental = b.source != nil

	return b, nil
}

func setupConnection(host string, insecure bool) (*grpc.ClientConn, error) {
	if insecure {
		return grpc.Dial(host,
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(x.GrpcMaxSize),
				grpc.MaxCallSendMsgSize(x.GrpcMaxSize)),
			grpc.WithInsecure(),
			grpc.WithBlock(),
			grpc.WithTimeout(10*time.Second))
	}

	tlsConf.ConfigType = x.TLSClientConfig
	tlsConf.CertRequired = false
	tlsCfg, _, err := x.GenerateTLSConfig(tlsConf)
	if err != nil {
		return nil, err
	}

	return grpc.Dial(host,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(x.GrpcMaxSize),
			grpc.MaxCallSendMsgSize(x.GrpcMaxSize)),
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second))
}
