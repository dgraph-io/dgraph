/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

func evalCompare(cmp string, lv, rv int64) bool {
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
