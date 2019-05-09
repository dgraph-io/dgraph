/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/dgo/x"
	"github.com/golang/glog"
)

// WriteFileSync is the same as bufio.WriteFile, but syncs the data before closing.
func WriteFileSync(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}

// WalkPathFunc walks the directory 'dir' and collects all path names matched by
// func f. If the path is a directory, it will set the bool argument to true.
// Returns empty string slice if nothing found, otherwise returns all matched path names.
func WalkPathFunc(dir string, f func(string, bool) bool) []string {
	var list []string
	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if f(path, fi.IsDir()) {
			list = append(list, path)
		}
		return nil
	})
	if err != nil {
		glog.Errorf("Error while scanning %q: %s", dir, err)
	}
	return list
}

// FindFilesFunc walks the directory 'dir' and collects all file names matched by
// func f. It will skip over directories.
// Returns empty string slice if nothing found, otherwise returns all matched file names.
func FindFilesFunc(dir string, f func(string) bool) []string {
	return WalkPathFunc(dir, func(path string, isdir bool) bool {
		return !isdir && f(path)
	})
}

// FindDataFiles returns a list of data files as a string array. If str is a comma-separated list
// of paths, it returns that list. If str is a single path that is not a directory, it returns that
// path. If str is a directory, it returns the files in it that have one of the extensions in ext.
func FindDataFiles(str string, ext []string) []string {
	if len(str) == 0 {
		return []string{}
	}

	list := strings.Split(str, ",")
	if len(list) == 1 && list[0] != "-" {
		// make sure the file or directory exists,
		// and recursively search for files if it's a directory

		fi, err := os.Stat(str)
		if os.IsNotExist(err) {
			glog.Errorf("File or directory does not exist: %s", str)
			return []string{}
		}
		x.Check(err)

		if fi.IsDir() {
			matchFn := func(f string) bool {
				for _, e := range ext {
					if strings.HasSuffix(f, e) {
						return true
					}
				}
				return false
			}
			list = FindFilesFunc(str, matchFn)
		}
	}

	return list
}

// IsMissingOrEmptyDir returns true if the path either does not exist
// or is a directory that is empty.
func IsMissingOrEmptyDir(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}

	if !fi.IsDir() {
		return false, nil
	}

	file, err := os.Open(path)
	defer file.Close()
	if err != nil {
		return false, err
	}

	_, err = file.Readdir(1)
	if err == nil {
		return false, nil
	} else if err != io.EOF {
		return false, err
	}

	return true, nil
}
