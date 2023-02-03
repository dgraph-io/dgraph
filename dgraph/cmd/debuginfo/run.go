/*
 * Copyright 2019-2022 Dgraph Labs, Inc. and Contributors
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

package debuginfo

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/spf13/cobra"

	"github.com/dgraph-io/dgraph/x"
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
		debugInfoCmd.directory, err = ioutil.TempDir("/tmp", "dgraph-debuginfo")
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
