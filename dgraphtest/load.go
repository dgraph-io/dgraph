/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package dgraphtest

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/x"
)

type LiveOpts struct {
	RdfFiles       []string
	SchemaFiles    []string
	GqlSchemaFiles []string
}

func readGzFile(sf string) ([]byte, error) {
	fd, err := os.Open(sf)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening file [%v]", sf)
	}
	defer func() {
		if err := fd.Close(); err != nil {
			log.Printf("[WARNING] error closing file [%v]: %v", sf, err)
		}
	}()

	gr, err := gzip.NewReader(fd)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating a gzip reader for file [%v]", sf)
	}
	defer func() {
		if err := gr.Close(); err != nil {
			log.Printf("[WARNING] error closing gzip reader for file [%v]: %v", sf, err)
		}
	}()

	data, err := io.ReadAll(gr)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading file content [%v]", sf)
	}
	return data, nil
}

func setDQLSchema(c *LocalCluster, files []string) error {
	gc, cleanup, err := c.Client()
	if err != nil {
		return errors.WithStack(err)
	}
	defer cleanup()
	if err := gc.LoginIntoNamespace(context.Background(),
		DefaultUser, DefaultPassword, x.GalaxyNamespace); err != nil {
		return errors.WithStack(err)
	}

	for _, sf := range files {
		data, err := readGzFile(sf)
		if err != nil {
			return err
		}
		if err := gc.SetupSchema(string(data)); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func setGraphQLSchema(c *LocalCluster, files []string) error {
	hc, err := c.HTTPClient()
	if err != nil {
		return errors.WithStack(err)
	}
	if err := hc.LoginIntoNamespace(DefaultUser, DefaultPassword, x.GalaxyNamespace); err != nil {
		return errors.WithStack(err)
	}

	for _, sf := range files {
		data, err := readGzFile(sf)
		if err != nil {
			return err
		}
		// if there is no GraphQL schema in the cluster,
		// the GQL file only has empty [].
		if len(data) < 10 {
			continue
		}
		if _, err := hc.UpdateGQLSchema(string(data)); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// LiveLoad runs the live loader with provided options
func (c *LocalCluster) LiveLoad(opts LiveOpts) error {
	if err := setDQLSchema(c, opts.SchemaFiles); err != nil {
		return err
	}
	if err := setGraphQLSchema(c, opts.GqlSchemaFiles); err != nil {
		return err
	}

	var alphaURLs []string
	for i, aa := range c.alphas {
		url, err := aa.alphaURL(c)
		if err != nil {
			return errors.Wrapf(err, "error finding URL to %vth alpha", i)
		}
		alphaURLs = append(alphaURLs, url)
	}
	zeroURL, err := c.zeros[0].zeroURL(c)
	if err != nil {
		return errors.Wrap(err, "error finding URL to 0th zero")
	}

	args := []string{
		"live",
		"--files", strings.Join(opts.RdfFiles, ","),
		"--alpha", strings.Join(alphaURLs, ","),
		"--zero", zeroURL,
	}
	if c.conf.acl {
		args = append(args, "--creds", fmt.Sprintf("user=%s;password=%s;namespace=%d",
			DefaultUser, DefaultPassword, x.GalaxyNamespace))
	}
	if c.conf.encryption {
		args = append(args, "--encryption", fmt.Sprintf("key-file=%v", encKeyPath))
	}

	log.Printf("[INFO] running live loader with args: [%v]", strings.Join(args, " "))
	cmd := exec.Command(filepath.Join(c.tempBinDir, "dgraph"), args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "error running live loader: %v", string(out))
	} else {
		log.Printf("[INFO] ==== output for live loader ====")
		log.Println(string(out))
	}
	return nil
}

// LiveLoadFromExport runs the live loader from the output of dgraph export
// The exportDir is the directory present inside the container. This function
// first copies all the files on the host and then runs the live loader.
func (c *LocalCluster) LiveLoadFromExport(exportDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	exportDirHost, err := os.MkdirTemp("", "dgraph-export")
	if err != nil {
		return errors.Wrap(err, "error creating temp dir for exported data")
	}

	// First, we need to copy the exported data from the container to host
	ts, _, err := c.dcli.CopyFromContainer(ctx, c.alphas[0].cid(), exportDir)
	if err != nil {
		return errors.Wrapf(err, "error copying export dir from container [%v]", c.alphas[0].cname())
	}
	defer func() {
		if err := ts.Close(); err != nil {
			log.Printf("[WARNING] error closing tared stream from docker cp for [%v]", c.alphas[0].cname())
		}
	}()

	// .rdf.gz, .schema.gz,.gql_schema.gz
	var rdfFiles, schemaFiles, gqlSchemaFiles []string
	tr := tar.NewReader(ts)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "error reading file in tared stream: [%+v]", header)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		fileName := filepath.Base(header.Name)
		hostFile := filepath.Join(exportDirHost, fileName)
		switch {
		case strings.HasSuffix(fileName, ".rdf.gz"):
			rdfFiles = append(rdfFiles, hostFile)
		case strings.HasSuffix(fileName, ".schema.gz"):
			schemaFiles = append(schemaFiles, hostFile)
		case strings.HasSuffix(fileName, ".gql_schema.gz"):
			gqlSchemaFiles = append(gqlSchemaFiles, hostFile)
		default:
			return errors.Errorf("found unexpected file in export: %v", fileName)
		}

		fd, err := os.Create(hostFile) //nolint: G305
		if err != nil {
			return errors.Wrapf(err, "error creating file [%v]", hostFile)
		}
		defer func() {
			if err := fd.Close(); err != nil {
				log.Printf("[WARNING] error closing file while docker cp: [%+v]", header)
			}
		}()
		if _, err := io.Copy(fd, tr); err != nil {
			return errors.Wrapf(err, "error writing to [%v] from: [%+v]", fd.Name(), header)
		}
	}

	opts := LiveOpts{
		RdfFiles:       rdfFiles,
		SchemaFiles:    schemaFiles,
		GqlSchemaFiles: gqlSchemaFiles,
	}
	if err := c.LiveLoad(opts); err != nil {
		return errors.Wrapf(err, "error running live loader: %v", err)
	}
	return nil
}
