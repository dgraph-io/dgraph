/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"github.com/hypermodeinc/dgraph/v25/types"
)

func couldApplyAggregatorOn(agrtr string, typ types.TypeID) bool {
	if !typ.IsScalar() {
		return false
	}
	switch agrtr {
	case "min", "max":
		return typ == types.IntID ||
			typ == types.FloatID ||
			typ == types.DateTimeID ||
			typ == types.StringID ||
			typ == types.DefaultID
	case "sum", "avg":
		return typ == types.IntID ||
			typ == types.FloatID ||
			typ == types.VFloatID
	default:
		return false
	}
}
