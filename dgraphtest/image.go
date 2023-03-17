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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

func (c *LocalCluster) dgraphImage() string {
	return "dgraph/dgraph:local"
}

func (c *LocalCluster) setupBinary() error {
	isFileThere, err := fileExists(filepath.Join(binDir, fmt.Sprintf(binaryName, c.conf.version)))
	if err != nil {
		return err
	}

	if isFileThere {
		return copyBinary(binDir, c.tempBinDir, c.conf.version)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()
	repo, err := git.PlainOpen(repoDir)
	if err != nil && err == git.ErrRepositoryNotExists {
		glog.Infof("cloning dgraph repo")
		repo, err = git.PlainCloneContext(ctx, repoDir, false, &git.CloneOptions{URL: dgraphRepoUrl})
		if err != nil {
			return errors.Wrap(err, "error while cloning dgraph git repo")
		}
	} else if err != nil {
		return errors.Wrap(err, "error while opening git repo")
	} else {
		if err := repo.Fetch(&git.FetchOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "error while fetching git repo")
		}
	}

	hash, err := repo.ResolveRevision(plumbing.Revision(c.conf.version))
	if err != nil {
		return errors.Wrap(err, "error while getting refrence hash")
	}
	if err := checkoutGitRepo(repo, hash); err != nil {
		return err
	}

	if err := buildDgraphBinary(repoDir, binDir, c.conf.version); err != nil {
		return err
	}
	return copyBinary(binDir, c.tempBinDir, c.conf.version)
}

func checkoutGitRepo(repo *git.Repository, hash *plumbing.Hash) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return errors.Wrap(err, "error while getting git repo work tree")
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: *hash}); err != nil {
		return errors.Wrap(err, fmt.Sprintf("error while checking out git repo with hash [%v]", hash.String()))
	}
	return nil
}

func buildDgraphBinary(dir, binaryDir, version string) error {
	glog.Infof("building dgraph binary")

	cmd := exec.Command("make", "dgraph")
	cmd.Dir = filepath.Join(dir, "dgraph")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "error while building dgraph binary")
	}
	if err := copy(filepath.Join(dir, "dgraph", "dgraph"),
		filepath.Join(binaryDir, fmt.Sprintf(binaryName, version))); err != nil {
		return errors.Wrap(err, "error while copying binary")
	}

	return nil
}

func copyBinary(fromDir, toDir, version string) error {
	if err := copy(filepath.Join(fromDir, fmt.Sprintf(binaryName, version)),
		filepath.Join(toDir, "dgraph")); err != nil {
		return errors.Wrap(err, "error while copying binary into tempBinDir")
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
	srcStat, _ := source.Stat()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("error while opening file [%s]", src))
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	if err := os.Chmod(dst, srcStat.Mode()); err != nil {
		return err
	}
	_, err = io.Copy(destination, source)
	return err
}
