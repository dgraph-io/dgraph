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
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"
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
		Check(err)

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

// ErrMissingDir is thrown by IsMissingOrEmptyDir if the given path is a
// missing or empty directory.
var ErrMissingDir = errors.Errorf("missing or empty directory")

// IsMissingOrEmptyDir returns true if the path either does not exist
// or is a directory that is empty.
func IsMissingOrEmptyDir(path string) (err error) {
	var fi os.FileInfo
	fi, err = os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = ErrMissingDir
			return
		}
		return
	}

	if !fi.IsDir() {
		return
	}

	var file *os.File
	file, err = os.Open(path)
	if err != nil {
		return
	}
	defer func() {
		cerr := file.Close()
		if err == nil {
			err = cerr
		}
	}()

	_, err = file.Readdir(1)
	if err == nil {
		return
	} else if err != io.EOF {
		return
	}

	err = ErrMissingDir
	return
}

// WriteGroupIdFile writes the given group ID to the group_id file inside the given
// postings directory.
func WriteGroupIdFile(pdir string, group_id uint32) error {
	if group_id == 0 {
		return errors.Errorf("ID written to group_id file must be a positive number")
	}

	groupFile := filepath.Join(pdir, GroupIdFileName)
	f, err := os.OpenFile(groupFile, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil
	}
	if _, err := f.WriteString(strconv.Itoa(int(group_id))); err != nil {
		return err
	}
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	return nil
}

// ReadGroupIdFile reads the file at the given path and attempts to retrieve the
// group ID stored in it.
func ReadGroupIdFile(pdir string) (uint32, error) {
	path := filepath.Join(pdir, GroupIdFileName)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if info.IsDir() {
		return 0, errors.Errorf("Group ID file at %s is a directory", path)
	}

	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}

	groupId, err := strconv.ParseUint(strings.TrimSpace(string(contents)), 0, 32)
	if err != nil {
		return 0, err
	}
	return uint32(groupId), nil
}
