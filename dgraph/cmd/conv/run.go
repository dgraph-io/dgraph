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

package conv

import (
	"fmt"
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
				fmt.Println(err)
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
