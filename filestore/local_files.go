/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package filestore

import (
	"bufio"
	"io"
	"os"

	"github.com/hypermodeinc/dgraph/v25/chunker"
	"github.com/hypermodeinc/dgraph/v25/x"
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

func (*localFiles) ChunkReader(file string, key x.Sensitive) (*bufio.Reader, func()) {
	return chunker.FileReader(file, key)
}

var _ FileStore = (*localFiles)(nil)
