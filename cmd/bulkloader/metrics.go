package main

import "expvar"

var (
	NumBadgerWrites     = expvar.NewInt("bulkloader_pending_badger_writes")
	NumReducers         = expvar.NewInt("bulkloader_reducers")
	NumQueuedReduceJobs = expvar.NewInt("bulkloader_queued_reduce_jobs")
)
