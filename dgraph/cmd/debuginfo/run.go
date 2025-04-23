/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package debuginfo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/spf13/cobra"

	"github.com/hypermodeinc/dgraph/v25/x"
)

type debugInfoCmdOpts struct {
	alphaAddr   string
	zeroAddr    string
	archive     bool
	directory   string
	seconds     uint32
	metricTypes []string
}

var (
	DebugInfo    x.SubCommand
	debugInfoCmd = debugInfoCmdOpts{}
)

var metricMap = map[string]string{
	"jemalloc":     "/debug/jemalloc",
	"state":        "/state",
	"health":       "/health",
	"vars":         "/debug/vars",
	"metrics":      "/metrics",
	"heap":         "/debug/pprof/heap",
	"goroutine":    "/debug/pprof/goroutine?debug=2",
	"threadcreate": "/debug/pprof/threadcreate",
	"block":        "/debug/pprof/block",
	"mutex":        "/debug/pprof/mutex",
	"cpu":          "/debug/pprof/profile",
	"trace":        "/debug/pprof/trace",
}

var metricList = []string{
	"heap",
	"cpu",
	"state",
	"health",
	"jemalloc",
	"trace",
	"metrics",
	"vars",
	"trace",
	"goroutine",
	"block",
	"mutex",
	"threadcreate",
}

func init() {
	DebugInfo.Cmd = &cobra.Command{
		Use:   "debuginfo",
		Short: "Generate debug information on the current node",
		Run: func(cmd *cobra.Command, args []string) {
			if err := collectDebugInfo(); err != nil {
				glog.Errorf("error while collecting dgraph debug info: %s", err)
				os.Exit(1)
			}
		},
		Annotations: map[string]string{"group": "debug"},
	}

	DebugInfo.EnvPrefix = "DGRAPH_AGENT_DEBUGINFO"
	DebugInfo.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flags := DebugInfo.Cmd.Flags()
	flags.StringVarP(&debugInfoCmd.alphaAddr, "alpha", "a", "localhost:8080",
		"Address of running dgraph alpha.")
	flags.StringVarP(&debugInfoCmd.zeroAddr, "zero", "z", "", "Address of running dgraph zero.")
	flags.StringVarP(&debugInfoCmd.directory, "directory", "d", "",
		"Directory to write the debug info into.")
	flags.BoolVarP(&debugInfoCmd.archive, "archive", "x", true,
		"Whether to archive the generated report")
	flags.Uint32VarP(&debugInfoCmd.seconds, "seconds", "s", 30,
		"Duration for time-based metric collection.")
	flags.StringSliceVarP(&debugInfoCmd.metricTypes, "metrics", "m", metricList,
		"List of metrics & profile to dump in the report.")

}

func collectDebugInfo() (err error) {
	if debugInfoCmd.directory == "" {
		debugInfoCmd.directory, err = os.MkdirTemp("/tmp", "dgraph-debuginfo")
		if err != nil {
			return fmt.Errorf("error while creating temporary directory: %s", err)
		}
	} else {
		err = os.MkdirAll(debugInfoCmd.directory, 0644)
		if err != nil {
			return err
		}
	}
	glog.Infof("using directory %s for debug info dump.", debugInfoCmd.directory)

	collectDebug()

	if debugInfoCmd.archive {
		return archiveDebugInfo()
	}
	return nil
}

func collectDebug() {
	if debugInfoCmd.alphaAddr != "" {
		filePrefix := filepath.Join(debugInfoCmd.directory, "alpha_")

		saveMetrics(debugInfoCmd.alphaAddr, filePrefix, debugInfoCmd.seconds, debugInfoCmd.metricTypes)

	}

	if debugInfoCmd.zeroAddr != "" {
		filePrefix := filepath.Join(debugInfoCmd.directory, "zero_")

		saveMetrics(debugInfoCmd.zeroAddr, filePrefix, debugInfoCmd.seconds, debugInfoCmd.metricTypes)

	}
}

func archiveDebugInfo() error {
	archivePath, err := createGzipArchive(debugInfoCmd.directory)
	if err != nil {
		return fmt.Errorf("error while archiving debuginfo directory: %s", err)
	}

	glog.Infof("Debuginfo archive successful: %s", archivePath)

	if err = os.RemoveAll(debugInfoCmd.directory); err != nil {
		glog.Warningf("error while removing debuginfo directory: %s", err)
	}
	return nil
}
