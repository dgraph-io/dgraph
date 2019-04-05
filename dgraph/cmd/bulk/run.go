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

package bulk

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // http profiler
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var Bulk x.SubCommand

var defaultOutDir = "./out"

func init() {
	Bulk.Cmd = &cobra.Command{
		Use:   "bulk",
		Short: "Run Dgraph bulk loader",
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Bulk.Conf).Stop()
			run()
		},
	}
	Bulk.EnvPrefix = "DGRAPH_BULK"

	flag := Bulk.Cmd.Flags()
	flag.StringP("files", "f", "",
		"Location of *.rdf(.gz) or *.json(.gz) file(s) to load.")
	flag.StringP("schema", "s", "",
		"Location of schema file.")
	flag.String("format", "",
		"Specify file format (rdf or json) instead of getting it from filename.")
	flag.String("out", defaultOutDir,
		"Location to write the final dgraph data directories.")
	flag.Bool("replace_out", false,
		"Replace out directory and its contents if it exists.")
	flag.String("tmp", "tmp",
		"Temp directory used to use for on-disk scratch space. Requires free space proportional"+
			" to the size of the RDF file and the amount of indexing used.")
	flag.IntP("num_go_routines", "j", runtime.NumCPU(),
		"Number of worker threads to use (defaults to the number of logical CPUs).")
	flag.Int64("mapoutput_mb", 64,
		"The estimated size of each map file output. Increasing this increases memory usage.")
	flag.Bool("skip_map_phase", false,
		"Skip the map phase (assumes that map output files already exist).")
	flag.Bool("cleanup_tmp", true,
		"Clean up the tmp directory after the loader finishes. Setting this to false allows the"+
			" bulk loader can be re-run while skipping the map phase.")
	flag.Int("shufflers", 1,
		"Number of shufflers to run concurrently. Increasing this can improve performance, and "+
			"must be less than or equal to the number of reduce shards.")
	flag.Bool("version", false, "Prints the version of Dgraph Bulk Loader.")
	flag.BoolP("store_xids", "x", false, "Generate an xid edge for each node.")
	flag.StringP("zero", "z", "localhost:5080", "gRPC address for Dgraph zero")
	// TODO: Potentially move http server to main.
	flag.String("http", "localhost:8080",
		"Address to serve http (pprof).")
	flag.Bool("ignore_errors", false, "ignore line parsing errors in rdf files")
	flag.Int("map_shards", 1,
		"Number of map output shards. Must be greater than or equal to the number of reduce "+
			"shards. Increasing allows more evenly sized reduce shards, at the expense of "+
			"increased memory usage.")
	flag.Int("reduce_shards", 1,
		"Number of reduce shards. This determines the number of dgraph instances in the final "+
			"cluster. Increasing this potentially decreases the reduce stage runtime by using "+
			"more parallelism, but increases memory usage.")
	flag.String("custom_tokenizers", "",
		"Comma separated list of tokenizer plugins")
	flag.Bool("new_uids", false,
		"Ignore UIDs in load files and assign new ones.")
}

func run() {
	opt := options{
		DataFiles:        Bulk.Conf.GetString("files"),
		DataFormat:       Bulk.Conf.GetString("format"),
		SchemaFile:       Bulk.Conf.GetString("schema"),
		OutDir:           Bulk.Conf.GetString("out"),
		ReplaceOutDir:    Bulk.Conf.GetBool("replace_out"),
		TmpDir:           Bulk.Conf.GetString("tmp"),
		NumGoroutines:    Bulk.Conf.GetInt("num_go_routines"),
		MapBufSize:       int64(Bulk.Conf.GetInt("mapoutput_mb")),
		SkipMapPhase:     Bulk.Conf.GetBool("skip_map_phase"),
		CleanupTmp:       Bulk.Conf.GetBool("cleanup_tmp"),
		NumShufflers:     Bulk.Conf.GetInt("shufflers"),
		Version:          Bulk.Conf.GetBool("version"),
		StoreXids:        Bulk.Conf.GetBool("store_xids"),
		ZeroAddr:         Bulk.Conf.GetString("zero"),
		HttpAddr:         Bulk.Conf.GetString("http"),
		IgnoreErrors:     Bulk.Conf.GetBool("ignore_errors"),
		MapShards:        Bulk.Conf.GetInt("map_shards"),
		ReduceShards:     Bulk.Conf.GetInt("reduce_shards"),
		CustomTokenizers: Bulk.Conf.GetString("custom_tokenizers"),
		NewUids:          Bulk.Conf.GetBool("new_uids"),
	}

	x.PrintVersion()
	if opt.Version {
		os.Exit(0)
	}
	if opt.SchemaFile == "" {
		fmt.Fprint(os.Stderr, "Schema file must be specified.\n")
		os.Exit(1)
	} else if _, err := os.Stat(opt.SchemaFile); err != nil && os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Schema path(%v) does not exist.\n", opt.SchemaFile)
		os.Exit(1)
	}
	if opt.DataFiles == "" {
		fmt.Fprint(os.Stderr, "RDF or JSON file(s) location must be specified.\n")
		os.Exit(1)
	} else if _, err := os.Stat(opt.DataFiles); err != nil && os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Data path(%v) does not exist.\n", opt.DataFiles)
		os.Exit(1)
	}
	if opt.ReduceShards > opt.MapShards {
		fmt.Fprintf(os.Stderr, "Invalid flags: reduce_shards(%d) should be <= map_shards(%d)\n",
			opt.ReduceShards, opt.MapShards)
		os.Exit(1)
	}
	if opt.NumShufflers > opt.ReduceShards {
		fmt.Fprintf(os.Stderr, "Invalid flags: shufflers(%d) should be <= reduce_shards(%d)\n",
			opt.NumShufflers, opt.ReduceShards)
		os.Exit(1)
	}
	if opt.CustomTokenizers != "" {
		for _, soFile := range strings.Split(opt.CustomTokenizers, ",") {
			tok.LoadCustomTokenizer(soFile)
		}
	}

	opt.MapBufSize <<= 20 // Convert from MB to B.

	optBuf, err := json.MarshalIndent(&opt, "", "\t")
	x.Check(err)
	fmt.Println(string(optBuf))

	maxOpenFilesWarning()

	go func() {
		log.Fatal(http.ListenAndServe(opt.HttpAddr, nil))
	}()

	// Make sure it's OK to create or replace the directory specified with the --out option.
	// It is always OK to create or replace the default output directory.
	if opt.OutDir != defaultOutDir && !opt.ReplaceOutDir {
		missingOrEmpty, err := x.IsMissingOrEmptyDir(opt.OutDir)
		x.CheckfNoTrace(err)
		if !missingOrEmpty {
			fmt.Fprintf(os.Stderr, "Output directory exists and is not empty."+
				" Use --replace_out to overwrite it.\n")
			os.Exit(1)
		}
	}

	// Delete and recreate the output dirs to ensure they are empty.
	x.Check(os.RemoveAll(opt.OutDir))
	for i := 0; i < opt.ReduceShards; i++ {
		dir := filepath.Join(opt.OutDir, strconv.Itoa(i), "p")
		x.Check(os.MkdirAll(dir, 0700))
		opt.shardOutputDirs = append(opt.shardOutputDirs, dir)
	}

	// Create a directory just for bulk loader's usage.
	if !opt.SkipMapPhase {
		x.Check(os.RemoveAll(opt.TmpDir))
		x.Check(os.MkdirAll(opt.TmpDir, 0700))
	}
	if opt.CleanupTmp {
		defer os.RemoveAll(opt.TmpDir)
	}

	loader := newLoader(opt)
	if !opt.SkipMapPhase {
		loader.mapStage()
		mergeMapShardsIntoReduceShards(opt)
	}
	loader.reduceStage()
	loader.writeSchema()
	loader.cleanup()
}

func maxOpenFilesWarning() {
	const (
		red    = "\x1b[31m"
		green  = "\x1b[32m"
		yellow = "\x1b[33m"
		reset  = "\x1b[0m"
	)
	maxOpenFiles, err := queryMaxOpenFiles()
	if err != nil || maxOpenFiles < 1e6 {
		fmt.Println(green + "\nThe bulk loader needs to open many files at once. This number depends" +
			" on the size of the data set loaded, the map file output size, and the level" +
			" of indexing. 100,000 is adequate for most data set sizes. See `man ulimit` for" +
			" details of how to change the limit.")
		if err != nil {
			fmt.Printf(red+"Nonfatal error: max open file limit could not be detected: %v\n"+reset, err)
		} else {
			fmt.Printf(yellow+"Current max open files limit: %d\n"+reset, maxOpenFiles)
		}
		fmt.Println()
	}
}
