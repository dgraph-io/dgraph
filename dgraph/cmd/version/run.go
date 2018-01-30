/*
 * Copyright (C) 2018 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
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
