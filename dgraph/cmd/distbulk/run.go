/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package distbulk

import (
    "os"
    "os/exec"
	"syscall"
	"github.com/spf13/cobra"
    "github.com/dgraph-io/dgraph/x"
)


var (
	Distbulk      x.SubCommand
)

func init() {
	Distbulk.Cmd = &cobra.Command{
		Use:   "distbulk",
		Short: "Run Dgraph distributed MapReduce bulk loader",
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Distbulk.Conf).Stop()
			run()
		},
	}
	Distbulk.EnvPrefix = "DGRAPH_DISTBULK"

	flag := Distbulk.Cmd.Flags()
	flag.StringP("rdfs", "r", "",
		"Directory containing *.rdf or *.rdf.gz files to load.")
	flag.StringP("schema_file", "s", "",
		"Location of schema file to load.")
	flag.String("out", "out",
		"Location to write the final dgraph data directories.")
	// flag.String("tmp", "tmp",
	// 	"Temp directory used to use for on-disk scratch space. Requires free space proportional"+
	// 		" to the size of the RDF file and the amount of indexing used.")
	// flag.IntP("num_go_routines", "j", runtime.NumCPU(),
	// 	"Number of worker threads to use (defaults to the number of logical CPUs)")
	// flag.Int64("mapoutput_mb", 64,
	// 	"The estimated size of each map file output. Increasing this increases memory usage.")
	flag.Bool("expand_edges", true,
		"Generate edges that allow nodes to be expanded using _predicate_ or expand(...). "+
			"Disable to increase loading speed.")
	// flag.Bool("skip_map_phase", false,
	// 	"Skip the map phase (assumes that map output files already exist).")
	// flag.Bool("cleanup_tmp", true,
	// 	"Clean up the tmp directory after the loader finishes. Setting this to false allows the"+
	// 		" bulk loader can be re-run while skipping the map phase.")
	// flag.Int("shufflers", 1,
	// 	"Number of shufflers to run concurrently. Increasing this can improve performance, and "+
	// 		"must be less than or equal to the number of reduce shards.")
	flag.BoolP("store_xids", "x", false, "Generate an xid edge for each node.")
	// flag.StringP("zero", "z", "localhost:5080", "gRPC address for Dgraph zero")
	// flag.String("http", "localhost:8080",
	// 	"Address to serve http (pprof).")
	// flag.Bool("ignore_errors", false, "ignore line parsing errors in rdf files")
	// flag.Int("map_shards", 1,
	// 	"Number of map output shards. Must be greater than or equal to the number of reduce "+
	// 		"shards. Increasing allows more evenly sized reduce shards, at the expense of "+
	// 		"increased memory usage.")
	// flag.Int("reduce_shards", 1,
	// 	"Number of reduce shards. This determines the number of dgraph instances in the final "+
	// 		"cluster. Increasing this potentially decreases the reduce stage runtime by using "+
	// 		"more parallelism, but increases memory usage.")
}

func run() {
    binary, lookErr := exec.LookPath("mapreduce")
    if lookErr != nil {
        panic(lookErr)
    }

    args := []string{binary, "--distributed"}
    env := os.Environ()
    execErr := syscall.Exec(binary, args, env)
    if execErr != nil {
        panic(execErr)
    }
}
