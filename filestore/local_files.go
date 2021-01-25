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
