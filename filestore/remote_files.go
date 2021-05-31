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
	"strings"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/minio/minio-go/v6"
)

type remoteFiles struct {
	mc *x.MinioClient
}

func (rf *remoteFiles) Open(path string) (io.ReadCloser, error) {
	url, err := url.Parse(path)
	if err != nil {
		return nil, err
	}

	bucket, prefix := rf.mc.ParseBucketAndPrefix(url.Path)
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

		c := make(chan struct{})
		defer close(c)

		bucket, prefix := rf.mc.ParseBucketAndPrefix(url.Path)
		for obj := range rf.mc.ListObjectsV2(bucket, prefix, true, c) {
			if hasAnySuffix(obj.Key, ext) {
				paths = append(paths, bucket+"/"+obj.Key)
			}
		}
	}
	return
}

func (rf *remoteFiles) ChunkReader(file string, key x.Sensitive) (*bufio.Reader, func()) {
	url, err := url.Parse(file)
	x.Check(err)

	bucket, prefix := rf.mc.ParseBucketAndPrefix(url.Path)

	obj, err := rf.mc.GetObject(bucket, prefix, minio.GetObjectOptions{})
	x.Check(err)

	return chunker.StreamReader(url.Path, key, obj)
}

var _ FileStore = (*remoteFiles)(nil)
