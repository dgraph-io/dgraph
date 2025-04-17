/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package conv

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hypermodeinc/dgraph/v25/x"
)

// Conv is the sub-command invoked when running "dgraph conv".
var Conv x.SubCommand

var opt struct {
	geo     string
	out     string
	geopred string
}

func init() {
	Conv.Cmd = &cobra.Command{
		Use:   "conv",
		Short: "Dgraph Geo file converter",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Conv.Conf).Stop()
			if err := run(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
		Annotations: map[string]string{"group": "tool"},
	}
	Conv.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flag := Conv.Cmd.Flags()
	flag.StringVar(&opt.geo, "geo", "", "Location of geo file to convert")
	flag.StringVar(&opt.out, "out", "output.rdf.gz", "Location of output rdf.gz file")
	flag.StringVar(&opt.geopred, "geopred", "loc", "Predicate to use to store geometries")
	x.Check(Conv.Cmd.MarkFlagRequired("geo"))
}

func run() error {
	return convertGeoFile(opt.geo, opt.out)
}
