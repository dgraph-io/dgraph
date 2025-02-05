//go:build windows
// +build windows

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import "github.com/pkg/errors"

func QueryMaxOpenFiles() (int, error) {
	return 0, errors.New("Cannot detect max open files on this platform")
}
