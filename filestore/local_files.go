/*
 * Copyright 2017-2021 Dgraph Labs, Inc. and Contributors
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

package filestore

import (
	"bufio"
	"io"
	"os"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/x"
)

type localFiles struct {
}

func (*localFiles) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (*localFiles) Exists(path string) bool {
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

func (*localFiles) FindDataFiles(str string, ext []string) []string {
	return x.FindDataFiles(str, ext)
}

func (*localFiles) ChunkReader(file string, key x.SensitiveByteSlice) (*bufio.Reader, func()) {
	return chunker.FileReader(file, key)
}

var _ FileStore = (*localFiles)(nil)
