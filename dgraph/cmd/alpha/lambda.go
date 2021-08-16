// +build oss

/*
 * Copyright 2021 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package alpha

import (
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
)

func setupLambdaServer(closer *z.Closer) {
	// If lambda-url is set, then don't launch the lambda servers from dgraph.
	if len(x.Config.GraphQL.GetString("lambda-url")) > 0 {
		return
	}

	num := int(x.Config.GraphQL.GetUint32("lambda-cnt"))
	if num == 0 {
		return
	}
	glog.Fatalf("Cannot setup lambda server using lambda-cnt flag: %v", x.ErrNotSupported)
}
