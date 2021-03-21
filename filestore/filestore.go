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
	"net/url"

	"github.com/dgraph-io/dgraph/x"
)

// FileStore represents a file or directory of files that are either stored
// locally or on minio/s3
type FileStore interface {
	// Similar to os.Open
	Open(path string) (io.ReadCloser, error)
	Exists(path string) bool
	FindDataFiles(str string, ext []string) []string
	ChunkReader(file string, key x.Sensitive) (*bufio.Reader, func())
}

// NewFileStore returns a new file storage. If remote, it's backed by an x.MinioClient
func NewFileStore(path string) FileStore {
	url, err := url.Parse(path)
	x.Check(err)

	if url.Scheme == "minio" || url.Scheme == "s3" {
		mc, err := x.NewMinioClient(url, nil)
		x.Check(err)

		return &remoteFiles{mc}
	}

	return &localFiles{}
}

// Open takes a single path and returns a io.ReadCloser, similar to os.Open
func Open(path string) (io.ReadCloser, error) {
	return NewFileStore(path).Open(path)
}

// Exists returns false if the file doesn't exist. For remote storage, true does
// not guarantee existence
func Exists(path string) bool {
	return NewFileStore(path).Exists(path)
}
