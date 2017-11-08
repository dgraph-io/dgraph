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

package bulk

import "expvar"

var (
	NumBadgerWrites     = expvar.NewInt("dgraph-bulk-loader_badger_writes_pending")
	NumReducers         = expvar.NewInt("dgraph-bulk-loader_num_reducers_total")
	NumQueuedReduceJobs = expvar.NewInt("dgraph-bulk-loader_reduce_queue_size")
)
