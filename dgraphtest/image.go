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
	"github.com/pkg/errors"
)

// var (
// 	homePath = os.Getenv("HOME") + "/dgraph"
// 	repoDir  = homePath + "/binaries"
// )

// const (
// 	relativeDir   = "binaries"
// 	dgraphRepoUrl = "https://github.com/dgraph-io/dgraph.git"
// 	cloneTimeout  = 10 * time.Minute
// )

func (c *LocalCluster) dgraphImage() string {
	return "dgraph/dgraph:local"
}

func (c *LocalCluster) setupBinary(version string, logger Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()
	repo, err := git.PlainOpen(repoDir)
	if err != nil && err == git.ErrRepositoryNotExists {
		logger.Logf("cloning repo")
		repo, err = git.PlainCloneContext(ctx, repoDir, false, &git.CloneOptions{
			URL: dgraphRepoUrl,
		})
		if err != nil {
			return errors.Wrap(err, "error while checking out dgraph git repo")
		}
	} else if err != nil {
		return errors.Wrap(err, "error while opening git repo")
	} else {
		if err := repo.Fetch(&git.FetchOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "error while fetching git repo")
		}
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(version))
	if err != nil {
		return err
	}
	if err := checkoutGitRepo(repo, hash); err != nil {
		return err
	}
	// make dgraph of specific version
	logger.Logf("making binary")
	if err := makeDgraphBinary(repoDir, binDir, version); err != nil {
		return err
	}
	if err := copy(filepath.Join(binDir, "dgraph_")+version, filepath.Join(c.tempBinDir, "dgraph")); err != nil {
		return errors.Wrap(err, "error while copying dgraph binary into temp dir")
	}
	return nil
}

func checkoutGitRepo(repo *git.Repository, hash *plumbing.Hash) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return errors.Wrap(err, "error while getting git repo work tree")
	}
	if err = worktree.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(hash.String())}); err != nil {
		return errors.Wrap(err, "error while checking out git repo")
	}
	return nil
}

func makeDgraphBinary(dir, binaryDir, version string) error {
	cmd := exec.Command("make", "dgraph")
	cmd.Dir = filepath.Join(dir, "dgraph")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "error getting while building dgraph binary")
	}
	if err := copy(filepath.Join(dir, "dgraph", "dgraph"), filepath.Join(binaryDir, "dgraph_")+version); err != nil {
		return errors.Wrap(err, "error while copying binary")
	}
	return nil
}

func flipVersion(upgradeVersion, tempDir string) error {
	if err := copy(filepath.Join(binDir, "dgraph_")+upgradeVersion, filepath.Join(tempDir, "dgraph")); err != nil {
		return errors.Wrap(err, "error while copying dgraph binary into temp dir")
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
	defer destination.Close()
	srcStat, _ := source.Stat()
	if err := os.Chmod(dst, srcStat.Mode()); err != nil {
		return err
	}
	_, err = io.Copy(destination, source)
	return err
}
