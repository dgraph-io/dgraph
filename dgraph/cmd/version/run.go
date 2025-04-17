/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package version

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hypermodeinc/dgraph/v25/x"
)

// Version is the sub-command invoked when running "dgraph version".
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
		Annotations: map[string]string{"group": "default"},
	}
	Version.Cmd.SetHelpTemplate(x.NonRootTemplate)
}
