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

package z

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/dgraph-io/dgraph/x"
)

// These are exported so they can also be set directly from outside this package.
var (
	ShowOutput  bool = os.Getenv("DEBUG_SHOW_OUTPUT") != ""
	ShowError   bool = os.Getenv("DEBUG_SHOW_ERROR") != ""
	ShowCommand bool = os.Getenv("DEBUG_SHOW_COMMAND") != ""
)

// Pipeline runs several commands such that the output of one command becomes the input of the next.
// The first argument should be an two-dimensional array containing the commands.
// TODO: allow capturing output, sending to terminal, etc
func Pipeline(cmds [][]string) error {
	var p io.ReadCloser
	var numCmds = len(cmds)

	cmd := make([]*exec.Cmd, numCmds)

	// Run all commands in parallel, connecting stdin of each to the stdout of the previous.
	for i, c := range cmds {
		lastCmd := i == numCmds-1
		if ShowCommand {
			fmt.Fprintf(os.Stderr, "%+v", c)
		}

		cmd[i] = exec.Command(c[0], c[1:]...)
		cmd[i].Stdin = p
		if !lastCmd {
			p, _ = cmd[i].StdoutPipe()
		}

		if ShowOutput {
			cmd[i].Stderr = os.Stderr
			if lastCmd {
				cmd[i].Stdout = os.Stdout
			}
		} else if ShowError {
			cmd[i].Stderr = os.Stderr
		}

		if ShowCommand {
			if lastCmd {
				fmt.Fprintf(os.Stderr, "\n")
			} else {
				fmt.Fprintf(os.Stderr, "\n| ")
			}
		}

		err := cmd[i].Start()
		x.Check(err)
	}

	// Make sure to properly reap all spawned processes, but only save the error from the
	// earliest stage of the pipeline.
	var err error
	for i := range cmds {
		e := cmd[i].Wait()
		if e != nil && err == nil {
			err = e
		}
	}

	return err
}
