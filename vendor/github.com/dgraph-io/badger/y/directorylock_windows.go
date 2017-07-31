/*
 * Copyright 2017 Dgraph Labs, Inc. and Contributors
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

package y

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// DirectoryLockGuard holds a lock on the directory.
type DirectoryLockGuard struct {
	path string
}

// AcquireDirectoryLock acquires exclusive access to a directory.
func AcquireDirectoryLock(dirPath string, pidFileName string) (*DirectoryLockGuard, error) {
	// Convert to absolute path so that Release still works even if we do an unbalanced
	// chdir in the meantime.
	absLockFilePath, err := filepath.Abs(filepath.Join(dirPath, pidFileName))
	if err != nil {
		return nil, errors.Wrap(err, "Cannot get absolute path for pid lock file")
	}

	f, err := os.OpenFile(absLockFilePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return nil, errors.Wrapf(err,
			"Cannot create pid lock file %q.  Another process is using this Badger database",
			absLockFilePath)
	}
	_, err = fmt.Fprintf(f, "%d\n", os.Getpid())
	closeErr := f.Close()
	if err != nil {
		return nil, errors.Wrap(err, "Cannot write to pid lock file")
	}
	if closeErr != nil {
		return nil, errors.Wrap(closeErr, "Cannot close pid lock file")
	}
	return &DirectoryLockGuard{path: absLockFilePath}, nil
}

// Release removes the directory lock.
func (g *DirectoryLockGuard) Release() error {
	path := g.path
	g.path = ""
	return os.Remove(path)
}
