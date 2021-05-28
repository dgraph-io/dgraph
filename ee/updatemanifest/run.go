// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package updatemanifest

import (
	"log"
	"net/url"
	"os"

	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	logger = log.New(os.Stderr, "", 0)
	// UpdateManifest is the sub-command invoked when running "dgraph update_manifest".
	UpdateManifest x.SubCommand
)

var opt struct {
	location string
	key      []byte
}

func init() {
	UpdateManifest.Cmd = &cobra.Command{
		Use:   "update_manifest",
		Short: "Run the Dgraph update tool to update the manifest from 2103 to 2105.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := run(); err != nil {
				logger.Fatalf("%v\n", err)
			}
		},
		Annotations: map[string]string{"group": "tool"},
	}
	UpdateManifest.EnvPrefix = "DGRAPH_UPDATE_MANIFEST"
	UpdateManifest.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flag := UpdateManifest.Cmd.Flags()
	flag.StringVarP(&opt.location, "location", "l", "",
		`Sets the location of the backup. Both file URIs and s3 are supported.
		This command will take care of all the full + incremental backups present in the location.`)
	ee.RegisterEncFlag(flag)
}

func run() error {
	keys, err := ee.GetKeys(UpdateManifest.Conf)
	if err != nil {
		return err
	}
	opt.key = keys.EncKey
	uri, err := url.Parse(opt.location)
	if err != nil {
		return errors.Wrapf(err, "while parsing location")
	}
	handler, err := x.NewUriHandler(uri, nil)
	if err != nil {
		return errors.Wrapf(err, "while creating uri handler")
	}
	masterManifest, err := worker.GetManifestNoUpgrade(handler, uri)
	if err != nil {
		return errors.Wrapf(err, "while getting manifest")
	}

	update := func(manifest *worker.Manifest) {
		for gid, preds := range manifest.Groups {
			parsedPreds := preds[:0]
			for _, pred := range preds {
				attr, err := x.AttrFrom2103(pred)
				if err != nil {
					parsedPreds = append(parsedPreds, pred)
					logger.Printf("Unable to parse the pred: %v", pred)
					continue
				}
				parsedPreds = append(parsedPreds, attr)
			}
			manifest.Groups[gid] = parsedPreds
		}
		for _, op := range manifest.DropOperations {
			if op.DropOp == pb.DropOperation_ATTR {
				attr, err := x.AttrFrom2103(op.DropValue)
				if err != nil {
					logger.Printf("Unable to parse the drop operation %+v pred: %v",
						op, []byte(op.DropValue))
					continue
				}
				op.DropValue = attr
			}
		}
		// As we have made the required changes to the manifest, we should update the version too.
		manifest.Version = 2105
	}

	// Update the master manifest with the changes for drop operations and group predicates.
	for _, manifest := range masterManifest.Manifests {
		if manifest.Version == 2103 {
			update(manifest)
		}
	}

	// Rewrite the master manifest.
	return errors.Wrap(worker.CreateManifest(handler, uri, masterManifest), "rewrite failed")
}
