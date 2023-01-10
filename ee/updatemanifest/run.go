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
	"encoding/binary"
	"log"
	"net/url"
	"os"
	"strings"

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
		Short: "Run the Dgraph update tool to update the manifest from v21.03 to latest.",
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

// Invalid bytes are replaced with the Unicode replacement rune.
// See https://golang.org/pkg/encoding/json/#Marshal
const replacementRune = rune('\ufffd')

func parseNsAttr(attr string) (uint64, string, error) {
	if strings.ContainsRune(attr, replacementRune) {
		return 0, "", errors.New("replacement char found")
	}
	return binary.BigEndian.Uint64([]byte(attr[:8])), attr[8:], nil
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
	handler, err := worker.NewUriHandler(uri, nil)
	if err != nil {
		return errors.Wrapf(err, "while creating uri handler")
	}
	masterManifest, err := worker.GetManifest(handler, uri)
	if err != nil {
		return errors.Wrapf(err, "while getting manifest")
	}

	// Update the master manifest with the changes for drop operations and group predicates.
	for _, manifest := range masterManifest.Manifests {
		for gid, preds := range manifest.Groups {
			parsedPreds := preds[:0]
			for _, pred := range preds {
				ns, attr, err := parseNsAttr(pred)
				if err != nil {
					logger.Printf("Unable to parse the pred: %v", pred)
					continue
				}
				parsedPreds = append(parsedPreds, x.NamespaceAttr(ns, attr))
			}
			manifest.Groups[gid] = parsedPreds
		}
		for _, op := range manifest.DropOperations {
			if op.DropOp == pb.DropOperation_ATTR {
				ns, attr, err := parseNsAttr(op.DropValue)
				if err != nil {
					logger.Printf("Unable to parse the drop operation %+v pred: %v",
						op, []byte(op.DropValue))
					continue
				}
				op.DropValue = x.NamespaceAttr(ns, attr)
			}
		}
	}

	// Rewrite the master manifest.
	return errors.Wrap(worker.CreateManifest(handler, uri, masterManifest), "rewrite failed")
}
