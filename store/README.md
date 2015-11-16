Results of benchmarking
------------------------

Using RocksDB

So, reading times are on the order of single unit microseconds, while writing
times with `Sync` set to true are ~30 milliseconds.

```
$ go test -run BenchmarkSet -v -bench .
PASS
BenchmarkGet_valsize100-6  	  500000	      2850 ns/op
--- BENCH: BenchmarkGet_valsize100-6
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
BenchmarkGet_valsize1000-6 	  500000	      3565 ns/op
--- BENCH: BenchmarkGet_valsize1000-6
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
BenchmarkGet_valsize10000-6	  200000	      8541 ns/op
--- BENCH: BenchmarkGet_valsize10000-6
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
	store_test.go:85: Wrote 100 keys.
BenchmarkSet_valsize100-6  	      50	  32932578 ns/op
BenchmarkSet_valsize1000-6 	      50	  28066678 ns/op
BenchmarkSet_valsize10000-6	      50	  28736228 ns/op
ok  	github.com/dgraph-io/dgraph/store	48.393s
```
