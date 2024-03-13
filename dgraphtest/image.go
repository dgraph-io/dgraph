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
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/mod/modfile"
)

func (c *LocalCluster) dgraphImage() string {
	return "dgraph/dgraph:local"
}

func (c *LocalCluster) setupBinary() error {
	if c.conf.customPlugins {
		race := false // Explicit var declaration to avoid confusion on the next line
		if err := c.GeneratePlugins(race); err != nil {
			return err
		}
	}
	if c.conf.version == localVersion {
		fromDir := filepath.Join(os.Getenv("GOPATH"), "bin")
		return copyBinary(fromDir, c.tempBinDir, c.conf.version)
	}

	isFileThere, err := fileExists(filepath.Join(binariesPath, fmt.Sprintf(binaryNameFmt, c.conf.version)))
	if err != nil {
		return err
	}
	if isFileThere {
		return copyBinary(binariesPath, c.tempBinDir, c.conf.version)
	}

	if err := runGitCheckout(c.conf.version); err != nil {
		return err
	}
	if err := buildDgraphBinary(repoDir, binariesPath, c.conf.version); err != nil {
		return err
	}
	return copyBinary(binariesPath, c.tempBinDir, c.conf.version)
}

func ensureDgraphClone() error {
	if _, err := os.Stat(repoDir); err != nil {
		return runGitClone()
	}

	if err := runGitStatus(); err != nil {
		if ierr := cleanupRepo(); ierr != nil {
			return ierr
		}
		return runGitClone()
	}

	// we do not return error if git fetch fails because maybe there are no changes
	// to pull and it doesn't make sense to fail right now. We can fail later when we
	// do not find the reference that we are looking for.
	if err := runGitFetch(); err != nil {
		log.Printf("[WARNING] error in fetching latest git changes: %v", err)
	}
	return nil
}

func cleanupRepo() error {
	return os.RemoveAll(repoDir)
}

func runGitClone() error {
	// The dgraph repo is already cloned for running the test. We can just create
	// a copy of this folder by running git clone using this already cloned dgraph
	// repo. After the quick clone, we update the original URL to point to the
	// GitHub dgraph repo and perform a "git fetch".
	cmd := exec.Command("git", "clone", baseRepoDir, repoDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "error cloning dgraph repo\noutput:%v", string(out))
	}

	cmd = exec.Command("git", "remote", "set-url", "origin", dgraphRepoUrl)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "error setting remote URL\noutput:%v", string(out))
	}

	// we do not return error if git fetch fails because maybe there are no changes
	// to pull and it doesn't make sense to fail right now. We can fail later when we
	// do not find the reference that we are looking for.
	if err := runGitFetch(); err != nil {
		log.Printf("[WARNING] error in fetching latest git changes in runGitClone: %v", err)
	}
	return nil
}

func runGitStatus() error {
	cmd := exec.Command("git", "status")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "error running git status\noutput:%v", string(out))
	}
	return nil
}

func runGitFetch() error {
	cmd := exec.Command("git", "fetch", "-p")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "error fetching latest changes\noutput:%v", string(out))
	}
	return nil
}

func runGitCheckout(gitRef string) error {
	cmd := exec.Command("git", "checkout", "-f", gitRef)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "error checking out gitRef [%v]\noutput:%v", gitRef, string(out))
	}
	return nil
}

func getHash(ref string) (string, error) {
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", errors.Wrapf(err, "error while running rev-parse on [%v]\noutput:%v", ref, string(out))
	} else {
		return strings.TrimSpace(string(out)), nil
	}
}

func buildDgraphBinary(dir, binaryDir, version string) error {
	log.Printf("[INFO] building dgraph binary for version [%v]", version)

	if err := fixGoModIfNeeded(); err != nil {
		return err
	}

	cmd := exec.Command("make", "dgraph")
	cmd.Dir = filepath.Join(dir, "dgraph")
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "error while building dgraph binary\noutput:%v", string(out))
	}
	if err := copy(filepath.Join(dir, "dgraph", "dgraph"),
		filepath.Join(binaryDir, fmt.Sprintf(binaryNameFmt, version))); err != nil {
		return errors.Wrap(err, "error while copying binary")
	}
	return nil
}

func copyBinary(fromDir, toDir, version string) error {
	binaryName := "dgraph"
	if version != localVersion {
		binaryName = fmt.Sprintf(binaryNameFmt, version)
	}
	fromPath := filepath.Join(fromDir, binaryName)
	toPath := filepath.Join(toDir, "dgraph")
	if err := copy(fromPath, toPath); err != nil {
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

func fixGoModIfNeeded() error {
	repoModFilePath := filepath.Join(repoDir, "go.mod")
	repoModFile, err := modfile.Parse(repoModFilePath, nil, nil)
	if err != nil {
		return errors.Wrapf(err, "error parsing mod file in repoDir [%v]", repoDir)
	}

	modFile, err := modfile.Parse("go.mod", nil, nil)
	if err != nil {
		return errors.Wrapf(err, "error while parsing go.mod file")
	}

	if len(modFile.Replace) == len(repoModFile.Replace) {
		return nil
	}

	repoModFile.Replace = modFile.Replace
	if data, err := repoModFile.Format(); err != nil {
		return errors.Wrapf(err, "error while formatting mod file")
	} else if err := os.WriteFile(repoModFilePath, data, 0644); err != nil {
		return errors.Wrapf(err, "error while writing to go.mod file")
	}
	return nil
}
