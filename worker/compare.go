/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

func EvalCompare(cmp string, lv, rv int64) bool {
	switch cmp {
	case "leq":
		return lv <= rv
	case "geq":
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
