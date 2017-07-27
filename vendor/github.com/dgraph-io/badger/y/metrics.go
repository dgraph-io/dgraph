/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package y

import (
	"expvar"
)

var (
	// These are cumulative
	NumGets         *expvar.Int
	NumPuts         *expvar.Int
	NumReads        *expvar.Int
	NumWrites       *expvar.Int
	NumBytesRead    *expvar.Int
	NumBytesWritten *expvar.Int
	NumMemtableGets *expvar.Int
	NumLSMGets      *expvar.Map
	NumLSMBloomHits *expvar.Map
)

func init() {
	NumReads = expvar.NewInt("badger_disk_reads_total")
	NumWrites = expvar.NewInt("badger_disk_writes_total")
	NumBytesRead = expvar.NewInt("badger_read_bytes")
	NumBytesWritten = expvar.NewInt("badger_written_bytes")
	NumGets = expvar.NewInt("badger_gets_total")
	NumPuts = expvar.NewInt("badger_puts_total")
	NumMemtableGets = expvar.NewInt("badger_memtable_gets_total")
	NumLSMGets = expvar.NewMap("badger_lsm_level_gets_total")
	NumLSMBloomHits = expvar.NewMap("badger_lsm_bloom_hits_total")
}
