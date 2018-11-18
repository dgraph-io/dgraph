/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package version

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dgraph-io/dgraph/x"
)

var Version x.SubCommand

func init() {
	Version.Cmd = &cobra.Command{
		Use:   "version",
		Short: "Prints the dgraph version details",
		Long:  "Version prints the dgraph version as reported by the build details.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(x.BuildDetails())
			os.Exit(0)
		},
	}
}
