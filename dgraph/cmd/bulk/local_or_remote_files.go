package bulk

import (
	"bufio"
	"context"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/dgraph-io/dgraph/chunker"
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
	FindDataFiles(str string, ext []string) []string
	ChunkReader(file string, key x.SensitiveByteSlice) (*bufio.Reader, func())
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

func (*localFiles) FindDataFiles(str string, ext []string) []string {
	return x.FindDataFiles(str, ext)
}

func (*localFiles) ChunkReader(file string, key x.SensitiveByteSlice) (*bufio.Reader, func()) {
	return chunker.FileReader(file, key)
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

func hasAnySuffix(str string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(str, suffix) {
			return true
		}
	}
	return false
}

func (rf *remoteFiles) FindDataFiles(str string, ext []string) (paths []string) {
	for _, dirPath := range strings.Split(str, ",") {
		url, err := url.Parse(dirPath)
		x.Check(err)

		bucket, prefix := minioclient.ParseBucketAndPrefix(url.Path)
		for obj := range rf.mc.ListObjectsV2(bucket, prefix, true, context.TODO().Done()) {
			if hasAnySuffix(obj.Key, ext) {
				paths = append(paths, bucket+"/"+obj.Key)
			}
		}
	}
	return
}

func (rf *remoteFiles) ChunkReader(file string, key x.SensitiveByteSlice) (*bufio.Reader, func()) {
	url, err := url.Parse(file)
	x.Check(err)

	bucket, prefix := minioclient.ParseBucketAndPrefix(url.Path)

	obj, err := rf.mc.GetObject(bucket, prefix, minio.GetObjectOptions{})
	x.Check(err)

	return chunker.StreamReader(url.Path, key, obj)
}

// OpenLocalOrRemoteFile takes a single path and returns a io.ReadCloser, similar to os.Open
func OpenLocalOrRemoteFile(path string) (io.ReadCloser, error) {
	return NewLocalOrRemoteFiles(path).Open(path)
}

func ExistsLocalOrRemoteFile(path string) bool {
	return NewLocalOrRemoteFiles(path).Exists(path)
}
