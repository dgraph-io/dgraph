package main

import "expvar"

var (
	NumBadgerWrites     = expvar.NewInt("dgraph-bulk-loader_badger_writes_pending")
	NumReducers         = expvar.NewInt("dgraph-bulk-loader_num_reducers_total")
	NumQueuedReduceJobs = expvar.NewInt("dgraph-bulk-loader_reduce_queue_size")
)
