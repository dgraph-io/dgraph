/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCidFileWriteDelete(t *testing.T) {
	InitSentry(false)
	ConfigureSentryScope("test")

	errString := "This is a test exception"
	WriteCidFile(errString)
	require.Equal(t, errString, readAndRemoveCidFile())
	require.NoFileExists(t, cidPath)
}
