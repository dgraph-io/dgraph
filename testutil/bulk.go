/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package testutil

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/x"
)

type LiveOpts struct {
	Alpha      string
	Zero       string
	RdfFile    string
	SchemaFile string
	Dir        string
	Env        []string
	Creds      *LoginParams
	ForceNs    int64
}

var (
	COVERAGE_FLAG         = "COVERAGE_OUTPUT"
	EXPECTED_COVERAGE_ENV = "--test.coverprofile=coverage.out"
)

func LiveLoad(opts LiveOpts) error {
	args := []string{
		"live",
		"--files", opts.RdfFile,
		"--schema", opts.SchemaFile,
		"--alpha", opts.Alpha,
		"--zero", opts.Zero,
	}
	if opts.Creds != nil {
		if opts.Creds.Namespace == x.GalaxyNamespace || opts.ForceNs != 0 {
			args = append(args, "--force-namespace", strconv.FormatInt(opts.ForceNs, 10))
		}
		args = append(args, "--creds")
		args = append(args, fmt.Sprintf("user=%s;password=%s;namespace=%d",
			opts.Creds.UserID, opts.Creds.Passwd, opts.Creds.Namespace))
	}
	liveCmd := exec.Command(DgraphBinaryPath(), args...)

	if opts.Dir != "" {
		liveCmd.Dir = opts.Dir
	}
	if opts.Env != nil {
		liveCmd.Env = append(os.Environ(), opts.Env...)
	}

	out, err := liveCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error %v\n", err)
		fmt.Printf("Output %v\n", string(out))
		return errors.Wrapf(err, string(out))
	}
	if CheckIfRace(out) {
		return errors.New("race condition detected. check logs for more details")
	}
	return nil
}

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

func freePort(port int) int {
	// Linux reuses ports in FIFO order. So a port that we listen on and then
	// release will be free for a long time.
	for {
		// p + 5080 and p + 9080 must lie within [20000, 60000]
		offset := 15000 + rand.Intn(30000)
		p := port + offset
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			listener.Close()
			return offset
		}
	}
}

func StartAlphas(compose string) error {
	cmd := exec.Command("docker-compose", "--compatibility", "-f", compose,
		"-p", DockerPrefix, "up", "-d", "--force-recreate")

	fmt.Println("Starting alphas with: ", cmd.String())

	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("Error while bringing up alpha node. Prefix: %s. Error: %v\n", DockerPrefix, err)
		fmt.Printf("Output %v\n", string(out))
		return err
	}

	for i := 1; i <= 6; i++ {
		in := GetContainerInstance(DockerPrefix, "alpha"+strconv.Itoa(i))
		err := in.BestEffortWaitForHealthy(8080)
		if err != nil {
			fmt.Printf("Error while checking alpha health %s. Error %v", in.Name, err)
			return err
		}
	}

	return nil
}

func StopAlphasForCoverage(composeFile string) {
	args := []string{"--compatibility", "-f", composeFile, "-p", DockerPrefix, "stop"}
	cmd := exec.CommandContext(context.Background(), "docker-compose", args...)
	fmt.Printf("Running: %s with %s\n", cmd, DockerPrefix)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error while bringing down cluster. Prefix: %s. Error: %v\n", DockerPrefix, err)
	}
}

func StopAlphasAndDetectRace(alphas []string) (raceDetected bool) {
	raceDetected = DetectRaceInAlphas(DockerPrefix)
	args := []string{"-p", DockerPrefix, "rm", "-f", "-s", "-v"}
	args = append(args, alphas...)
	cmd := exec.CommandContext(context.Background(), "docker-compose", args...)
	fmt.Printf("Running: %s with %s\n", cmd, DockerPrefix)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error while bringing down cluster. Prefix: %s. Error: %v\n", DockerPrefix, err)
	}
	return raceDetected
}
