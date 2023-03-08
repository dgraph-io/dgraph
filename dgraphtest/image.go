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
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"
)

var (
	tempDir  string
	homePath = os.Getenv("HOME") + "/data/dgraph"
	repoDir  = homePath + "/binaries"
)

const (
	relativeDir   = "binaries"
	dgraphRepoUrl = "https://github.com/dgraph-io/dgraph.git"
)

func (c *LocalCluster) dgraphImage() string {
	return "dgraph/dgraph:local"
}

func setupBinary(version string, logger Logger) error {
	var err error
	absPath, err := filepath.Abs(repoDir)
	if err != nil {
		return errors.Wrap(err, "error while getting absolute path")
	}
	repo, err := git.PlainOpen(absPath)
	if err != nil && err.Error() == "repository does not exist" {
		logger.Logf("cloning repo")
		repo, err = git.PlainCloneContext(context.Background(), absPath, false, &git.CloneOptions{
			URL: dgraphRepoUrl,
		})
		if err != nil {
			return errors.Wrap(err, "error getting while checking out git repo")

		}
	} else if err != nil {
		return errors.Wrap(err, "error while opening git repo")
	} else {
		if err := repo.Fetch(&git.FetchOptions{}); err != nil && err.Error() != "already up-to-date" {
			return errors.Wrap(err, "error while fetching git repo")
		}
	}
	tagRef, err := repo.Tag(version)
	if err != nil {
		return errors.Wrap(err, "error while getting tagref of version")
	}
	//checkout repo with specific tag
	if err := checkoutGitRepo(repo, tagRef.Hash(), "branch"+version); err != nil {
		return err
	}
	absPath, err = filepath.Abs(relativeDir)
	if err != nil {
		return errors.Wrap(err, "error while getting absolute path")
	}
	//make dgraph of specific version
	fmt.Println("making binary")
	if err := makeDgraphBinary(repoDir, absPath, version); err != nil {
		return err
	}
	err = copy(relativeDir+"/dgraph_"+version, tempDir+"/dgraph")
	if err != nil {
		return errors.Wrap(err, "error while copying dgraph binary into temp dir ")
	}
	fmt.Println("at the end")
	return nil
}

func checkoutGitRepo(repo *git.Repository, hash plumbing.Hash, branchName string) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return errors.Wrap(err, "error getting git repo work tree")

	}
	err = worktree.Checkout(&git.CheckoutOptions{
		Hash:   hash,
		Create: true,
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
	if err != nil && strings.Contains(err.Error(), "already exists") {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "error getting while checking out git repo")
	}
	return nil
}

func makeDgraphBinary(dir, binaryDir, version string) error {
	cmd := exec.Command("make", "dgraph")
	cmd.Dir = dir + "/dgraph"
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "error getting while building dgraph binary")
	}
	err := copy(dir+"/dgraph"+"/dgraph", binaryDir+"/dgraph_"+version)
	if err != nil {
		return errors.Wrap(err, "error while while copying binary")
	}
	return nil
}

func flipVersion(upgradeVersion string) error {
	fmt.Println("changing version")
	absPathBaseUp, err := filepath.Abs(relativeDir)
	if err != nil {
		return errors.Wrap(err, "error while getting absolute path of upgraded version dir")
	}
	err = copy(absPathBaseUp+"/dgraph_"+upgradeVersion, tempDir+"/dgraph")
	if err != nil {
		return errors.Wrap(err, "error while copying dgraph binary into temp dir ")
	}
	return nil
}

func copy(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !sourceFileStat.Mode().IsRegular() {
		return errors.Wrap(err, fmt.Sprintf("%s is not a regular file", src))
	}
	source, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("error while opening file [%s]", src))
	}
	defer source.Close()
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	srcStat, _ := source.Stat()
	if err := os.Chmod(dst, srcStat.Mode()); err != nil {
		return err
	}

	defer destination.Close()
	_, err = io.Copy(destination, source)
	return err
}
