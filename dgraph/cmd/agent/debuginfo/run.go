/*
 * Copyright 2019-2020 Dgraph Labs, Inc. and Contributors
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
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

type debugInfoCmdOpts struct {
	alphaAddr        string
	zeroAddr         string
	archive          bool
	directory        string
	infoDurationSecs uint32

	pprofProfiles []string
}

var (
	DebugInfo    x.SubCommand
	debugInfoCmd = debugInfoCmdOpts{}
)

func init() {
	DebugInfo.Cmd = &cobra.Command{
		Use:   "debuginfo",
		Short: "Generate debug info for dgraph on the current node.",
		Run: func(cmd *cobra.Command, args []string) {
			err := collectDebugInfo()
			if err != nil {
				glog.Errorf("error while collecting dgraph debug info: %s", err)
				os.Exit(1)
			}
		},
	}
	DebugInfo.EnvPrefix = "DGRAPH_AGENT_DEBUGINFO"

	flags := DebugInfo.Cmd.Flags()
	flags.StringVarP(&debugInfoCmd.alphaAddr, "alpha", "a", "localhost:8080",
		"Address of running dgraph alpha.")
	flags.StringVarP(&debugInfoCmd.zeroAddr, "zero", "z", "", "Address of running dgraph zero.")
	flags.StringVarP(&debugInfoCmd.directory, "directory", "d", "",
		"Directory to write the debug info into.")
	flags.BoolVarP(&debugInfoCmd.archive, "archive", "x", true,
		"whether or not to archive the agent info, this could come handy when we need to export "+
			"the dump.")
	flags.Uint32VarP(&debugInfoCmd.infoDurationSecs, "seconds", "s", 15,
		"Duration for time-based profile collection.")
	flags.StringSliceVarP(&debugInfoCmd.pprofProfiles, "profiles", "p", pprofProfileTypes,
		"list of pprof profiles to dump in debuginfo.")
}

func collectDebugInfo() (err error) {
	if debugInfoCmd.directory == "" {
		debugInfoCmd.directory, err = ioutil.TempDir("/tmp", "dgraph-debuginfo")
		if err != nil {
			return fmt.Errorf("error while creating temporary directory for debuginfo: %s", err)
		}
	} else {
		err = os.MkdirAll(debugInfoCmd.directory, 0644)
		if err != nil {
			return err
		}
	}
	glog.Infof("using directory %s for debug info dump.", debugInfoCmd.directory)

	collectPProfProfiles()

	if debugInfoCmd.archive {
		return archiveDebugInfo()
	}
	return nil
}

func collectPProfProfiles() {
	var duration time.Duration = time.Duration(debugInfoCmd.infoDurationSecs) * time.Second
	pc := newPprofCollector(debugInfoCmd.directory, duration, debugInfoCmd.pprofProfiles)
	if debugInfoCmd.alphaAddr != "" {
		pc.Collect(debugInfoCmd.alphaAddr, "alpha_")
	}

	if debugInfoCmd.zeroAddr != "" {
		pc.Collect(debugInfoCmd.zeroAddr, "zero_")
	}
}

func archiveDebugInfo() error {
	archivePath, err := createGzipArchive(debugInfoCmd.directory)
	if err != nil {
		return fmt.Errorf("error while archiving debuginfo directory: %s", err)
	}

	glog.Infof("Debuginfo archive successful: %s", archivePath)

	err = os.RemoveAll(debugInfoCmd.directory)
	if err != nil {
		glog.Warningf("error while removing debuginfo directory: %s", err)
	}
	return nil
}
