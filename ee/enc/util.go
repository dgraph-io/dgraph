//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package enc

import (
	"io"
)

// Eebuild indicates if this is a Enterprise build.
var EeBuild = false

// GetWriter returns the Writer as is for OSS Builds.
func GetWriter(_ []byte, w io.Writer) (io.Writer, error) {
	return w, nil
}

// GetReader returns the reader as is for OSS Builds.
func GetReader(_ []byte, r io.Reader) (io.Reader, error) {
	return r, nil
}
