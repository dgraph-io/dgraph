/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package version

import (
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
			x.PrintVersionOnly()
		},
	}
}
