/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package dgraphtest

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

var (
	COVERAGE_FLAG         = "COVERAGE_OUTPUT"
	EXPECTED_COVERAGE_ENV = "--test.coverprofile=coverage.out"
)

type BulkOpts struct {
	Zero          string
	Shards        int
	RdfFile       string
	SchemaFile    string
	GQLSchemaFile string
	Dir           string
	Env           []string
	Namespace     uint64
}

func BulkLoad(opts BulkOpts) error {

	var args []string

	if cc := os.Getenv(COVERAGE_FLAG); cc == EXPECTED_COVERAGE_ENV {
		args = append(args, "--test.coverprofile=coverage_bulk.out")
	}

	args = append(args, "bulk",
		"-f", opts.RdfFile,
		"-s", opts.SchemaFile,
		"-g", opts.GQLSchemaFile,
		"--http", "localhost:"+strconv.Itoa(freePort(0)),
		"--reduce_shards="+strconv.Itoa(opts.Shards),
		"--map_shards="+strconv.Itoa(opts.Shards),
		"--store_xids=true",
		"--zero", opts.Zero,
		"--force-namespace", strconv.FormatUint(opts.Namespace, 10))

	bulkCmd := exec.Command(DgraphBinaryPath(), args...)

	fmt.Println("Running: ", bulkCmd.Args)

	if opts.Dir != "" {
		bulkCmd.Dir = opts.Dir
	}

	if opts.Env != nil {
		bulkCmd.Env = append(os.Environ(), opts.Env...)
	}

	out, err := bulkCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error %v\n", err)
		fmt.Printf("Output %v\n", string(out))
		return err
	}

	if CheckIfRace(out) {
		return errors.New("race condition detected. check logs for more details")
	}
	return nil
}

func freePort(port int) int {
	// Linux reuses ports in FIFO order. So a port that we listen on and then
	// release will be free for a long time.
	for {
		// p + 5080 and p + 9080 must lie within [20000, 60000]
		offset := 15000 + rand.Intn(30000)
		p := port + offset
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			if err := listener.Close(); err != nil {
				glog.Warningf("error closing listener: %v", err)
			}
			return offset
		}
	}
}

func CheckIfRace(output []byte) bool {
	awkCmd := exec.Command("awk", "/WARNING: DATA RACE/{flag=1}flag;/==================/{flag=0}")
	awkCmd.Stdin = bytes.NewReader(output)
	out, err := awkCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error: while getting race content %v\n", err)
		return false
	}

	if len(out) > 0 {
		fmt.Printf("DATA RACE DETECTED %s\n", string(out))
		return true
	}
	return false
}
func MakeDirEmpty(dir []string) error {
	for _, d := range dir {
		_ = os.RemoveAll(d)
		err := os.MkdirAll(d, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}
