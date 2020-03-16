/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"errors"

	"github.com/dgraph-io/dgraph/x"
)

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
	x.Panic(errors.New("EvalCompare: unreachable"))
	return false
}
