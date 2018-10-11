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

package bulk

import "expvar"

var (
	NumBadgerWrites     = expvar.NewInt("dgraph-bulk-loader_badger_writes_pending")
	NumReducers         = expvar.NewInt("dgraph-bulk-loader_num_reducers_total")
	NumQueuedReduceJobs = expvar.NewInt("dgraph-bulk-loader_reduce_queue_size")
)
