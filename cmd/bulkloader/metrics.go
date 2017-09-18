package main

import "expvar"

var (
	NumBadgerWrites     = expvar.NewInt("bulkloader_badger_writes_pending")
	NumReducers         = expvar.NewInt("bulkloader_num_reducers_total")
	NumQueuedReduceJobs = expvar.NewInt("bulkloader_reduce_queue_size")
)
