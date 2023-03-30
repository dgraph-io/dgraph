/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package dgraphtest

import (
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	repoDir       string // repoDir to store cloned repository of dgraph
	binDir        string // binDir to store multiple binary versions
	encKeyPath    string
	aclSecretPath string
)

const (
	dgraphRepoUrl = "https://github.com/dgraph-io/dgraph.git"
	cloneTimeout  = 10 * time.Minute
)

func init() {
	// init log
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// setup paths
	_, thisFilePath, _, _ := runtime.Caller(0)
	basePath := strings.ReplaceAll(thisFilePath, "/paths.go", "")
	repoDir = filepath.Join(basePath, "repo")
	binDir = filepath.Join(basePath, "binaries")
	encKeyPath = filepath.Join(basePath, "data", "enc-key")
	aclSecretPath = filepath.Join(basePath, "data", "hmac-secret")
}
