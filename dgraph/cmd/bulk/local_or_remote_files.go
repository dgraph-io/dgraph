package bulk

import (
	"io"
	"net/url"
	"os"

	"github.com/dgraph-io/dgraph/minioclient"
	"github.com/dgraph-io/dgraph/x"
	"github.com/minio/minio-go/v6"
)

// LocalOrRemoteFiles represents a file or directory of files that are either stored
// locally or on minio/s3
type LocalOrRemoteFiles interface {
	// Similar to os.Open
	Open(path string) (io.ReadCloser, error)
	Exists(path string) bool
}

func NewLocalOrRemoteFiles(path string) LocalOrRemoteFiles {
	url, err := url.Parse(path)
	x.Check(err)

	if url.Scheme == "minio" || url.Scheme == "s3" {
		mc, err := minioclient.NewMinioClient(url, nil)
		x.Check(err)

		return &remoteFiles{mc}
	}

	return &localFiles{}
}

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

type remoteFiles struct {
	mc *minio.Client
}

func (rf *remoteFiles) Open(path string) (io.ReadCloser, error) {
	url, err := url.Parse(path)
	x.Check(err)

	bucket, prefix := minioclient.ParseBucketAndPrefix(url.Path)
	obj, err := rf.mc.GetObject(bucket, prefix, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// Checking if a file exists is a no-op in minio, since s3 cannot confirm if a directory exists
func (rf *remoteFiles) Exists(path string) bool {
	return true
}

// OpenLocalOrRemoteFile takes a single path and returns a io.ReadCloser, similar to os.Open
func OpenLocalOrRemoteFile(path string) (io.ReadCloser, error) {
	return NewLocalOrRemoteFiles(path).Open(path)
}

func ExistsLocalOrRemoteFile(path string) bool {
	return NewLocalOrRemoteFiles(path).Exists(path)
}
