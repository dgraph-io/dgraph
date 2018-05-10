/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

func EvalCompare(cmp string, lv, rv int64) bool {
	switch cmp {
	case "le":
		return lv <= rv
	case "ge":
		return lv >= rv
	case "lt":
		return lv < rv
	case "gt":
		return lv > rv
	case "eq":
		return lv == rv
	}
	panic("EvalCompare: unreachable")
}
