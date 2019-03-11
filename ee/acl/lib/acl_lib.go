// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package lib

import "github.com/dgraph-io/dgo/protos/api"

func GetCreateUserNQuads(userId string, password string) []*api.NQuad {
	return []*api.NQuad{
		{
			Subject:     "_:newuser",
			Predicate:   "dgraph.xid",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: userId}},
		},
		{
			Subject:     "_:newuser",
			Predicate:   "dgraph.password",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: password}},
		},
		{
			Subject:     "_:newuser",
			Predicate:   "type",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "User"}},
		},
	}
}
