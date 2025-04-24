//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"context"
	"testing"

	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
	"github.com/hypermodeinc/dgraph/v25/x"
)

func TestMain(m *testing.M) {
	dc = dgraphtest.NewComposeCluster()

	var err error
	var cleanup func()
	client, cleanup, err = dc.Client()
	x.Panic(err)
	defer cleanup()
	x.Panic(client.LoginIntoNamespace(context.Background(), dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	populateCluster(dc)
	m.Run()
}
