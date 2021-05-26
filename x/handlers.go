/*
 * Copyright 2018-2021 Dgraph Labs, Inc. and Contributors
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

package x

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/golang/glog"
	"github.com/minio/minio-go/v6"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/pkg/errors"
)

// UriHandler interface is implemented by URI scheme handlers.
// When adding new scheme handles, for example 'azure://', an object will implement
// this interface to supply Dgraph with a way to create or load backup files into DB.
// For all methods below, the URL object is parsed as described in `newHandler' and
// the Processor object has the DB, estimated tablets size, and backup parameters.
type UriHandler interface {
	// CreateDir creates a directory relative to the root path of the handler.
	CreateDir(path string) error
	// CreateFile creates a file relative to the root path of the handler. It also makes the
	// handler's descriptor to point to this file.
	CreateFile(path string) (io.WriteCloser, error)
	// DirExists returns true if the directory relative to the root path of the handler exists.
	DirExists(path string) bool
	// FileExists returns true if the file relative to the root path of the handler exists.
	FileExists(path string) bool
	// JoinPath appends the given path to the root path of the handler.
	JoinPath(path string) string
	// ListPaths returns a list of all the valid paths from the given root path. The given root path
	// should be relative to the handler's root path.
	ListPaths(path string) []string
	// Read reads the file at given relative path and returns the read bytes.
	Read(path string) ([]byte, error)
	// Rename renames the src file to the destination file.
	Rename(src, dst string) error
	// Stream would stream the path via an instance of io.ReadCloser. Close must be called at the
	// end to release resources appropriately.
	Stream(path string) (io.ReadCloser, error)
}

// NewUriHandler parses the requested URI and finds the corresponding UriHandler.
// If the passed credentials are not nil, they will be used to override the
// default credentials (only for backups to minio or S3).
// Target URI formats:
//   [scheme]://[host]/[path]?[args]
//   [scheme]:///[path]?[args]
//   /[path]?[args] (only for local or NFS)
//
// Target URI parts:
//   scheme - service handler, one of: "file", "s3", "minio"
//     host - remote address. ex: "dgraph.s3.amazonaws.com"
//     path - directory, bucket or container at target. ex: "/dgraph/backups/"
//     args - specific arguments that are ok to appear in logs.
//
// Global args (if supported by the handler):
//     secure - true|false turn on/off TLS.
//      trace - true|false turn on/off HTTP tracing.
//   compress - true|false turn on/off data compression.
//    encrypt - true|false turn on/off data encryption.
//
// Examples:
//   s3://dgraph.s3.amazonaws.com/dgraph/backups?secure=true
//   minio://localhost:9000/dgraph?secure=true
//   file:///tmp/dgraph/backups
//   /tmp/dgraph/backups?compress=gzip
//   https://dgraph.blob.core.windows.net/dgraph/backups
//   gs://dgraph/backups
func NewUriHandler(uri *url.URL, creds *MinioCredentials) (UriHandler, error) {
	switch uri.Scheme {
	case "file", "":
		return NewFileHandler(uri), nil
	case "minio", "s3":
		return NewS3Handler(uri, creds)
	case "gs":
		return NewGCSHandler(uri, creds)
	}

	if strings.HasSuffix(uri.Host, "blob.core.windows.net") {
		return NewAZSHandler(uri, creds)
	}

	return nil, errors.Errorf("Unable to handle url: %s", uri)
}

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	rootDir string
	prefix  string
}

func NewFileHandler(uri *url.URL) *fileHandler {
	h := &fileHandler{}
	h.rootDir, h.prefix = filepath.Split(uri.Path)
	return h
}

func (h *fileHandler) DirExists(path string) bool {
	path = h.JoinPath(path)
	stat, err := os.Stat(path)
	if err != nil {
		return false
	}
	return stat.IsDir()
}

func (h *fileHandler) FileExists(path string) bool {
	path = h.JoinPath(path)
	stat, err := os.Stat(path)
	if err != nil {
		return false
	}
	return stat.Mode().IsRegular()
}

func (h *fileHandler) Read(path string) ([]byte, error) {
	return ioutil.ReadFile(h.JoinPath(path))
}

func (h *fileHandler) JoinPath(path string) string {
	return filepath.Join(h.rootDir, h.prefix, path)
}
func (h *fileHandler) Stream(path string) (io.ReadCloser, error) {
	return os.Open(h.JoinPath(path))
}
func (h *fileHandler) ListPaths(path string) []string {
	path = h.JoinPath(path)
	return WalkPathFunc(path, func(path string, isDis bool) bool {
		return true
	})
}
func (h *fileHandler) CreateDir(path string) error {
	path = h.JoinPath(path)
	if err := os.MkdirAll(path, 0755); err != nil {
		return errors.Errorf("Create path failed to create path %s, got error: %v", path, err)
	}
	return nil
}

type fileSyncer struct {
	fp *os.File
}

func (fs *fileSyncer) Write(p []byte) (n int, err error) { return fs.fp.Write(p) }
func (fs *fileSyncer) Close() error {
	if err := fs.fp.Sync(); err != nil {
		return errors.Wrapf(err, "while syncing file: %s", fs.fp.Name())
	}
	err := fs.fp.Close()
	return errors.Wrapf(err, "while closing file: %s", fs.fp.Name())
}

func (h *fileHandler) CreateFile(path string) (io.WriteCloser, error) {
	path = h.JoinPath(path)
	fp, err := os.Create(path)
	return &fileSyncer{fp}, errors.Wrapf(err, "File handler failed to create file %s", path)
}

func (h *fileHandler) Rename(src, dst string) error {
	src = h.JoinPath(src)
	dst = h.JoinPath(dst)
	return os.Rename(src, dst)
}

// S3 Handler.

// s3Handler is used for 's3:' and 'minio:' URI schemes.
type s3Handler struct {
	bucketName, objectPrefix string
	creds                    *MinioCredentials
	uri                      *url.URL
	mc                       *MinioClient
}

// NewS3Handler creates a new session, checks valid bucket at uri.Path, and configures a
// minio client. It also fills in values used by the handler in subsequent calls.
// Returns a new S3 minio client, otherwise a nil client with an error.
func NewS3Handler(uri *url.URL, creds *MinioCredentials) (*s3Handler, error) {
	h := &s3Handler{
		creds: creds,
		uri:   uri,
	}
	mc, err := NewMinioClient(uri, creds)
	if err != nil {
		return nil, err
	}
	h.mc = mc
	h.bucketName, h.objectPrefix = mc.ParseBucketAndPrefix(uri.Path)
	return h, nil
}

func (h *s3Handler) CreateDir(path string) error { return nil }
func (h *s3Handler) DirExists(path string) bool  { return true }

func (h *s3Handler) FileExists(path string) bool {
	objectPath := h.getObjectPath(path)
	_, err := h.mc.StatObject(h.bucketName, objectPath, minio.StatObjectOptions{})
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return false
		} else {
			glog.Errorf("Failed to verify object existence: %v", err)
			return false
		}
	}
	return true
}

func (h *s3Handler) JoinPath(path string) string {
	return filepath.Join(h.bucketName, h.objectPrefix, path)
}

func (h *s3Handler) Read(path string) ([]byte, error) {
	objectPath := h.getObjectPath(path)
	var buf bytes.Buffer

	reader, err := h.mc.GetObject(h.bucketName, objectPath, minio.GetObjectOptions{})
	if err != nil {
		return buf.Bytes(), errors.Wrap(err, "Failed to read s3 object")
	}
	defer reader.Close()

	if _, err := buf.ReadFrom(reader); err != nil {
		return buf.Bytes(), errors.Wrap(err, "Failed to read the s3 object")
	}
	return buf.Bytes(), nil
}

func (h *s3Handler) Stream(path string) (io.ReadCloser, error) {
	objectPath := h.getObjectPath(path)
	reader, err := h.mc.GetObject(h.bucketName, objectPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return reader, nil
}

func (h *s3Handler) ListPaths(path string) []string {
	var paths []string
	done := make(chan struct{})
	defer close(done)
	path = h.getObjectPath(path)
	for object := range h.mc.ListObjects(h.bucketName, path, true, done) {
		paths = append(paths, object.Key)
	}
	return paths
}

type s3Writer struct {
	pwriter    *io.PipeWriter
	preader    *io.PipeReader
	bucketName string
	cerr       chan error
}

func (sw *s3Writer) Write(p []byte) (n int, err error) { return sw.pwriter.Write(p) }
func (sw *s3Writer) Close() error {
	if sw.pwriter == nil {
		return nil
	}
	if err := sw.pwriter.CloseWithError(nil); err != nil && err != io.EOF {
		glog.Errorf("Unexpected error when closing pipe: %v", err)
	}
	sw.pwriter = nil
	glog.V(2).Infof("Backup waiting for upload to complete.")
	return <-sw.cerr
}

// upload will block until it's done or an error occurs.
func (sw *s3Writer) upload(mc *MinioClient, object string) {
	f := func() error {
		start := time.Now()

		// We don't need to have a progress object, because we're using a Pipe. A write to Pipe
		// would block until it can be fully read. So, the rate of the writes here would be equal to
		// the rate of upload. We're already tracking progress of the writes in stream.Lists, so no
		// need to track the progress of read. By definition, it must be the same.
		//
		// PutObject would block until sw.preader returns EOF.
		n, err := mc.PutObject(sw.bucketName, object, sw.preader, -1, minio.PutObjectOptions{})
		glog.V(2).Infof("Backup sent %d bytes. Time elapsed: %s",
			n, time.Since(start).Round(time.Second))

		if err != nil {
			// This should cause Write to fail as well.
			glog.Errorf("Backup: Closing RW pipe due to error: %v", err)
			if err := sw.pwriter.Close(); err != nil {
				return err
			}
			if err := sw.preader.Close(); err != nil {
				return err
			}
		}
		return err
	}
	sw.cerr <- f()
}

func (h *s3Handler) CreateFile(path string) (io.WriteCloser, error) {
	objectPath := h.getObjectPath(path)
	glog.V(2).Infof("Sending data to %s blob %q ...", h.uri.Scheme, objectPath)

	sw := &s3Writer{
		bucketName: h.bucketName,
		cerr:       make(chan error, 1),
	}
	sw.preader, sw.pwriter = io.Pipe()
	go sw.upload(h.mc, objectPath)
	return sw, nil
}

func (h *s3Handler) Rename(srcPath, dstPath string) error {
	srcPath = h.getObjectPath(srcPath)
	dstPath = h.getObjectPath(dstPath)
	src := minio.NewSourceInfo(h.bucketName, srcPath, nil)
	dst, err := minio.NewDestinationInfo(h.bucketName, dstPath, nil, nil)
	if err != nil {
		return errors.Wrap(err, "Rename failed to create dstInfo")
	}
	// We try copying 100 times, if it still fails, then the user should manually rename.
	err = RetryUntilSuccess(100, time.Second, func() error {
		if err := h.mc.CopyObject(dst, src); err != nil {
			return errors.Wrapf(err, "While renaming object in s3, copy failed")
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = h.mc.RemoveObject(h.bucketName, srcPath)
	return errors.Wrap(err, "Rename failed to remove temporary file")
}

func (h *s3Handler) getObjectPath(path string) string {
	return filepath.Join(h.objectPrefix, path)
}

const AZSSeparator = '/'

type AZS struct {
	bucket      *azblob.ContainerURL
	client      *azblob.ServiceURL // Azure sdk client
	accountName string
	bucketName  string
	pathName    string
}

// Helper function to get the account, container and path of the destination folder from an Azure
// URL and adds it to azs.
func getAzDetailsFromUri(uri *url.URL, azs *AZS) (err error) {
	// azure url -> https://<account_name>.blob.core.windows.net/<container>/path/to/dest
	parts := strings.Split(uri.Host, ".")
	if len(parts) > 0 {
		azs.accountName = parts[0]
	} else {
		err = errors.Errorf("invalid azure host: %s", uri.Host)
		return
	}

	parts = strings.Split(uri.Path, string(filepath.Separator))
	if len(parts) > 1 {
		azs.bucketName = parts[1]
		azs.pathName = strings.Join(parts[2:], string(filepath.Separator))
	} else {
		err = errors.Errorf("invalid azure path: %s", uri.Path)
		return
	}

	return
}

// NewAZSHandler creates a new azure storage handler.
func NewAZSHandler(uri *url.URL, creds *MinioCredentials) (*AZS, error) {
	azs := &AZS{}
	if err := getAzDetailsFromUri(uri, azs); err != nil {
		return nil, errors.Wrapf(err, "while getting bucket details")
	}

	var azCreds azblob.Credential
	if creds.isAnonymous() {
		azCreds = azblob.NewAnonymousCredential()
	} else {
		// Override credentials from the Azure storage environment variables if specified
		key := os.Getenv("AZURE_STORAGE_KEY")
		if len(creds.SecretKey) > 0 {
			key = creds.SecretKey
		}

		if len(key) == 0 {
			return nil, errors.Errorf("Missing secret key for azure access.")
		}

		sharedkey, err := azblob.NewSharedKeyCredential(azs.accountName, key)
		if err != nil {
			return nil, errors.Wrap(err, "while creating sharedkey")
		}

		azCreds = sharedkey
	}

	// NewServiceURL only requires hostname and scheme.
	client := azblob.NewServiceURL(url.URL{
		Scheme: uri.Scheme,
		Host:   uri.Host,
	}, azblob.NewPipeline(azCreds, azblob.PipelineOptions{}))
	azs.client = &client

	bucket := azs.client.NewContainerURL(azs.bucketName)
	azs.bucket = &bucket

	// Verify that bucket exists.
	if _, err := azs.bucket.GetProperties(context.Background(),
		azblob.LeaseAccessConditions{}); err != nil {
		return nil, errors.Wrap(err, "while checking if bucket exists")
	}

	return azs, nil
}

// CreateDir creates a directory relative to the root path of the handler.
func (azs *AZS) CreateDir(path string) error {
	// Can't create a directory separately in azure. Folders are emulated with path separator.
	// Empty directories are not allowed.
	return nil
}

// CreateFile creates a file relative to the root path of the handler. It also makes the
// handler's descriptor to point to this file.
func (azs *AZS) CreateFile(path string) (io.WriteCloser, error) {
	azW := &azWriter{
		errCh: make(chan error, 1),
	}
	azW.pReader, azW.pWriter = io.Pipe()
	go azW.upload(azs.bucket, azs.JoinPath(path))
	return azW, nil
}

// DirExists returns true if the directory relative to the root path of the handler exists.
func (azs *AZS) DirExists(path string) bool {
	// Can't create a directory separately in azure. Folders are emulated with path separator.
	// Empty directories are not allowed.
	return true
}

// FileExists returns true if the file relative to the root path of the handler exists.
func (azs *AZS) FileExists(path string) bool {
	if _, err := azs.bucket.NewBlobURL(azs.JoinPath(path)).GetProperties(context.Background(),
		azblob.BlobAccessConditions{}, azblob.ClientProvidedKeyOptions{}); err != nil {
		glog.Errorf("while checking if file exists: %s", err)
		return false
	}
	return true
}

// JoinPath appends the given path to the root path of the handler.
func (azs *AZS) JoinPath(path string) string {
	return filepath.Join(azs.pathName, path)
}

// ListPaths returns a list of all the valid paths from the given root path. The given root path
// should be relative to the handler's root path.
func (azs *AZS) ListPaths(path string) []string {
	paths := []string{}
	marker := azblob.Marker{}
	for marker.NotDone() {
		blobList, err := azs.bucket.ListBlobsFlatSegment(context.Background(), marker,
			azblob.ListBlobsSegmentOptions{
				Prefix: azs.JoinPath(path),
			})
		if err != nil {
			glog.Errorf("while listing paths: %q", err)
			return nil
		}

		marker = blobList.NextMarker
		for _, blobinfo := range blobList.Segment.BlobItems {
			name := blobinfo.Name
			name = name[len(azs.pathName):]
			paths = append(paths, name)
		}
	}

	return paths
}

// Read reads the file at given relative path and returns the read bytes.
func (azs *AZS) Read(path string) ([]byte, error) {
	resp, err := azs.bucket.NewBlockBlobURL(azs.JoinPath(path)).Download(context.Background(), 0,
		azblob.CountToEnd, azblob.BlobAccessConditions{}, false, azblob.ClientProvidedKeyOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "while reading file")
	}

	buf := bytes.Buffer{}
	if _, err := buf.ReadFrom(resp.Body(azblob.RetryReaderOptions{})); err != nil {
		return nil, errors.Wrap(err, "while reading file")
	}
	return buf.Bytes(), nil
}

// Rename renames the src file to the destination file.
func (azs *AZS) Rename(src, dst string) error {
	ctx := context.Background()

	srcHandle := azs.bucket.NewBlockBlobURL(azs.JoinPath(src))
	resp, err := srcHandle.Download(ctx, 0,
		azblob.CountToEnd, azblob.BlobAccessConditions{}, false, azblob.ClientProvidedKeyOptions{})
	if err != nil {
		return errors.Wrap(err, "while reading file")
	}

	if _, err = azblob.UploadStreamToBlockBlob(ctx, resp.Body(azblob.RetryReaderOptions{}),
		azs.bucket.NewBlockBlobURL(azs.JoinPath(dst)),
		azblob.UploadStreamToBlockBlobOptions{}); err != nil {
		return errors.Wrapf(err, "while uploading")
	}

	if _, err = srcHandle.Delete(ctx, azblob.DeleteSnapshotsOptionInclude,
		azblob.BlobAccessConditions{}); err != nil {
		return errors.Wrapf(err, "while deleting file")
	}

	return nil
}

// Stream would stream the path via an instance of io.ReadCloser. Close must be called at the
// end to release resources appropriately.
func (azs *AZS) Stream(path string) (io.ReadCloser, error) {
	resp, err := azs.bucket.NewBlockBlobURL(azs.JoinPath(path)).Download(context.Background(), 0,
		azblob.CountToEnd, azblob.BlobAccessConditions{}, false, azblob.ClientProvidedKeyOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "while reading file")
	}

	return resp.Body(azblob.RetryReaderOptions{}), nil
}

type azWriter struct {
	pReader *io.PipeReader
	pWriter *io.PipeWriter
	errCh   chan error
}

// Write writes to the pipe writer.
func (w *azWriter) Write(p []byte) (n int, err error) {
	return w.pWriter.Write(p)
}

// Close calls close on the pipe writer and returns any erorrs encoutered in writing using the
// pipe reader.
func (w *azWriter) Close() error {
	if w == nil {
		return nil
	}

	if err := w.pWriter.Close(); err != nil {
		glog.Errorf("Unexpected error when closing pipe: %v", err)
	}
	w.pWriter = nil

	return <-w.errCh
}

// Helper function to process writes to azure. The function is run is a separate go-routine because
// the azure API requires a reader instead of a writer as an input. Any errors are returned to
// errCh of the azWriter, which are returned when close on the azWriter is called.
func (w *azWriter) upload(bucket *azblob.ContainerURL, absPath string) {
	f := func() error {
		ctx := context.Background()
		_, err := azblob.UploadStreamToBlockBlob(ctx, w.pReader, bucket.NewBlockBlobURL(absPath),
			azblob.UploadStreamToBlockBlobOptions{})

		return errors.Wrapf(err, "while uploading")
	}
	w.errCh <- f()
}

const GCSSeparator = '/'

type GCS struct {
	client     *storage.Client
	bucket     *storage.BucketHandle
	pathPrefix string
}

func NewGCSHandler(uri *url.URL, creds *MinioCredentials) (gcs *GCS, err error) {
	ctx := context.Background()

	var c *storage.Client
	// TODO(rohanprasad): Add support for API key, if it's sufficient to access storage bucket.
	if creds.isAnonymous() {
		if c, err = storage.NewClient(ctx, option.WithoutAuthentication()); err != nil {
			return nil, err
		}
	} else if creds.SecretKey != "" {
		f, err := os.Open(creds.SecretKey)
		if err != nil {
			return nil, err
		}

		data, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}

		c, err = storage.NewClient(ctx, option.WithCredentialsJSON(data))
		if err != nil {
			return nil, err
		}
	} else {
		// If no credentials are supplied, the library checks for environment variable
		// GOOGLE_APPLICATION_CREDENTIALS otherwise falls back to use the service account attached
		// to the resource running the code.
		// https://cloud.google.com/docs/authentication/production#automatically
		c, err = storage.NewClient(ctx)
		if err != nil {
			return nil, err
		}
	}

	gcs = &GCS{
		client:     c,
		pathPrefix: uri.Path,
	}

	if len(gcs.pathPrefix) > 0 && gcs.pathPrefix[0] == GCSSeparator {
		gcs.pathPrefix = gcs.pathPrefix[1:]
	}

	gcs.bucket = gcs.client.Bucket(uri.Host)
	if _, err := gcs.bucket.Attrs(ctx); err != nil {
		gcs.client.Close()
		return nil, errors.Wrapf(err, "while accessing bucket")
	}

	return gcs, nil
}

// CreateDir creates a directory relative to the root path of the handler.
func (gcs *GCS) CreateDir(path string) error {
	ctx := context.Background()

	// GCS uses a flat storage and provides an illusion of directories. To create a directory, file
	// name must be followed by '/'.
	dir := filepath.Join(gcs.pathPrefix, path, "") + string(GCSSeparator)
	glog.V(2).Infof("Creating dir: %q", dir)

	writer := gcs.bucket.Object(dir).NewWriter(ctx)
	if err := writer.Close(); err != nil {
		return errors.Wrapf(err, "while creating directory")
	}

	return nil
}

// CreateFile creates a file relative to the root path of the handler. It also makes the
// handler's descriptor to point to this file.
func (gcs *GCS) CreateFile(path string) (io.WriteCloser, error) {
	ctx := context.Background()

	writer := gcs.bucket.Object(gcs.JoinPath(path)).NewWriter(ctx)
	return writer, nil
}

// DirExists returns true if the directory relative to the root path of the handler exists.
func (gcs *GCS) DirExists(path string) bool {
	ctx := context.Background()

	absPath := gcs.JoinPath(path)

	// If there's no root specified we return true because we have ensured that the bucket exists.
	if len(absPath) == 0 {
		return true
	}

	// GCS doesn't has the concept of directories, it emulated the folder behaviour if the path is
	// suffixed with '/'.
	absPath += string(GCSSeparator)

	it := gcs.bucket.Objects(ctx, &storage.Query{
		Prefix: absPath,
	})

	if _, err := it.Next(); err == iterator.Done {
		return false
	} else if err == nil {
		return true
	} else {
		glog.Errorf("Error while checking if directory exists: %s", err)
		return false
	}
}

// FileExists returns true if the file relative to the root path of the handler exists.
func (gcs *GCS) FileExists(path string) bool {
	ctx := context.Background()

	obj := gcs.bucket.Object(gcs.JoinPath(path))
	if _, err := obj.Attrs(ctx); err == storage.ErrObjectNotExist {
		return false
	} else if err != nil {
		glog.Errorf("Error while checking if file exists: %s", err)
		return false
	}

	return true
}

// JoinPath appends the given path to the root path of the handler.
func (gcs *GCS) JoinPath(path string) string {
	if len(gcs.pathPrefix) == 0 {
		return path
	}

	if len(path) == 0 {
		return gcs.pathPrefix
	}

	return gcs.pathPrefix + string(GCSSeparator) + path
}

// ListPaths returns a list of all the valid paths from the given root path. The given root path
// should be relative to the handler's root path.
func (gcs *GCS) ListPaths(path string) []string {
	ctx := context.Background()

	absPath := gcs.JoinPath(path)
	if len(absPath) != 0 {
		absPath += string(GCSSeparator)
	}

	it := gcs.bucket.Objects(ctx, &storage.Query{
		Prefix: absPath,
	})

	paths := []string{}

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			glog.Errorf("Error while listing paths: %s", err)
		}

		if len(attrs.Name) > 0 {
			paths = append(paths, attrs.Name)
		} else if len(attrs.Prefix) > 0 {
			paths = append(paths, attrs.Prefix)
		}
	}

	return paths
}

// Read reads the file at given relative path and returns the read bytes.
func (gcs *GCS) Read(path string) ([]byte, error) {
	ctx := context.Background()
	reader, err := gcs.bucket.Object(gcs.JoinPath(path)).NewReader(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "while reading file")
	}
	defer reader.Close()

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrapf(err, "while reading file")
	}

	return data, nil
}

// Rename renames the src file to the destination file.
func (gcs *GCS) Rename(src, dst string) error {
	ctx := context.Background()

	srcObj := gcs.bucket.Object(gcs.JoinPath(src))
	dstObj := gcs.bucket.Object(gcs.JoinPath(dst))

	if _, err := dstObj.CopierFrom(srcObj).Run(ctx); err != nil {
		return errors.Wrapf(err, "while renaming file")
	}

	if err := srcObj.Delete(ctx); err != nil {
		return errors.Wrapf(err, "while renaming file")
	}

	return nil
}

// Stream would stream the path via an instance of io.ReadCloser. Close must be called at the
// end to release resources appropriately.
func (gcs *GCS) Stream(path string) (io.ReadCloser, error) {
	ctx := context.Background()
	reader, err := gcs.bucket.Object(gcs.JoinPath(path)).NewReader(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "while reading file")
	}

	return reader, nil
}
