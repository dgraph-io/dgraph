/*
 * Copyright 2019-2020 Dgraph Labs, Inc. and Contributors
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

package debuginfo

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
)

type tarWriter interface {
	io.Writer
	WriteHeader(hdr *tar.Header) error
}

type walker struct {
	baseDir  string
	debugDir string
	output   tarWriter
}

// walkPath function is called for each file present within the directory
// that walker is processing. The function operates in a best effort manner
// and tries to archive whatever it can without throwing an error.
func (w *walker) walkPath(path string, info os.FileInfo, err error) error {
	if err != nil {
		glog.Errorf("Error while walking path %s: %s", path, err)
		return nil
	}
	if info == nil {
		glog.Errorf("No file info available")
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		glog.Errorf("Failed to open %s: %s", path, err)
		return nil
	}
	defer file.Close()

	if info.IsDir() {
		if info.Name() == w.baseDir {
			return nil
		}
		glog.Errorf("Skipping directory %s", info.Name())
		return nil
	}

	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		glog.Errorf("Failed to prepare file info %s: %s", info.Name(), err)
		return nil
	}

	if w.baseDir != "" {
		header.Name = filepath.Join(w.baseDir, strings.TrimPrefix(path, w.debugDir))
	}

	if err := w.output.WriteHeader(header); err != nil {
		glog.Errorf("Failed to write header: %s", err)
		return nil
	}

	_, err = io.Copy(w.output, file)
	return err
}

// createArchive creates a gzipped tar archive for the directory provided
// by recursively traversing in the directory.
// The final tar is placed in the same directory with the name same to the
// archived directory.
func createArchive(debugDir string) (string, error) {
	archivePath := fmt.Sprintf("%s.tar", filepath.Base(debugDir))
	file, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	writer := tar.NewWriter(file)
	defer writer.Close()

	var baseDir string
	if info, err := os.Stat(debugDir); os.IsNotExist(err) {
		return "", err
	} else if err == nil && info.IsDir() {
		baseDir = filepath.Base(debugDir)
	}

	w := &walker{
		baseDir:  baseDir,
		debugDir: debugDir,
		output:   writer,
	}
	return archivePath, filepath.Walk(debugDir, w.walkPath)
}

// Creates a Gzipped tar archive of the directory provided as parameter.
func createGzipArchive(debugDir string) (string, error) {
	source, err := createArchive(debugDir)
	if err != nil {
		return "", err
	}

	reader, err := os.Open(source)
	if err != nil {
		return "", err
	}

	filename := filepath.Base(source)
	target := fmt.Sprintf("%s.gz", source)
	writer, err := os.Create(target)
	if err != nil {
		return "", err
	}
	defer writer.Close()

	archiver := gzip.NewWriter(writer)
	archiver.Name = filename
	defer archiver.Close()

	_, err = io.Copy(archiver, reader)
	if err != nil {
		return "", err
	}

	if err = os.Remove(source); err != nil {
		glog.Warningf("error while removing intermediate tar file: %s", err)
	}

	return target, nil
}
