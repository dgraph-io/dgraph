package main

import "expvar"

var (
	NumBadgerWrites = expvar.NewInt("bulkloader_badger_writes")
)
