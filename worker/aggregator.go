/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import "github.com/dgraph-io/dgraph/types"

func CouldApplyAggregatorOn(agrtr string, typ types.TypeID) bool {
	if !typ.IsScalar() {
		return false
	}
	switch agrtr {
	case "min", "max":
		return (typ == types.IntID ||
			typ == types.FloatID ||
			typ == types.DateTimeID ||
			typ == types.StringID ||
			typ == types.DefaultID)
	case "sum", "avg":
		return (typ == types.IntID ||
			typ == types.FloatID)
	default:
		return false
	}
}
