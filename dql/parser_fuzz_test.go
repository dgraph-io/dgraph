/*
 * Copyright 2017-2025 Hypermode Inc. and Contributors
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

package dql

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"testing"
)

const (
	corpusTarFile = "fuzz-data/corpus.tar.gz"
)

func FuzzTestParser(f *testing.F) {
	// add the corpus data to the test
	fd, err := os.Open(corpusTarFile)
	if err != nil {
		f.Fatalf("error opening corpus tar file: %v", err)
	}
	defer func() {
		if err := fd.Close(); err != nil {
			f.Logf("error closing corpus tar file: %v", err)
		}
	}()

	gzr, err := gzip.NewReader(fd)
	if err != nil {
		f.Fatalf("error reading corpus tar file: %v", err)
	}
	defer func() {
		if err := gzr.Close(); err != nil {
			f.Logf("error closing corpus tar file: %v", err)
		}
	}()
	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Fatalf("error while reading corpus tar file: %v", err)
		}

		switch header.Typeflag {
		case tar.TypeReg:
			buf := new(bytes.Buffer)
			if _, err := io.Copy(buf, tr); err != nil {
				f.Fatalf("error while copying file [%v]: %v", header.Name, err)
			}
			f.Add(buf.Bytes())
			f.Logf("adding input: %v", buf.String())
		}
	}

	f.Fuzz(func(t *testing.T, in []byte) {
		defer func() {
			if err := recover(); err != nil {
				t.Errorf("DQL parsing failed for input [%x] with error: [%v]", in, err)
			}
		}()

		_, _ = Parse(Request{Str: string(in)})
	})
}
