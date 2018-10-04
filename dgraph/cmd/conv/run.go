/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package conv

import (
	"os"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

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
				x.Println(err)
				os.Exit(1)
			}
		},
	}

	flag := Conv.Cmd.Flags()
	flag.StringVar(&opt.geo, "geo", "", "Location of geo file to convert")
	flag.StringVar(&opt.out, "out", "output.rdf.gz", "Location of output rdf.gz file")
	flag.StringVar(&opt.geopred, "geopred", "loc", "Predicate to use to store geometries")
	Conv.Cmd.MarkFlagRequired("geo")
}

func run() error {
	return convertGeoFile(opt.geo, opt.out)
}
