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

// FindFilesFunc walks the directory 'dir' and collects all file names matched by
// func f. It will skip over directories.
// Returns empty string slice if nothing found, otherwise returns all matched file names.
func FindFilesFunc(dir string, f func(string) bool) []string {
	var files []string
	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() && f(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		glog.Errorf("Error while scanning %q: %s", dir, err)
	}
	return files
}

// FindDataFiles returns a list of data files as a string array. If str is a comma-separated list
// of paths, it returns that list. If str is a single path that is not a directory, it returns that
// path. If str is a directory, it returns the files in it that have one of the extensions in ext.
func FindDataFiles(str string, ext []string) []string {
	if len(str) == 0 {
		return []string{}
	}

	list := strings.Split(str, ",")
	if len(list) == 1 {
		fi, err := os.Stat(str)
		if os.IsNotExist(err) {
			glog.Errorf("File or directory does not exist: %s", str)
			return []string{}
		}
		x.Check(err)

		if fi.IsDir() {
			match_fn := func(f string) bool {
				for _, x := range ext {
					if strings.HasSuffix(f, x) {
						return true
					}
				}
				return false
			}
			list = FindFilesFunc(str, match_fn)
		}
	}

	return list
}
