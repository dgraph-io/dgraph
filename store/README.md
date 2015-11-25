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

Also based on dgraph-io/experiments/db:

## BoltDB

Without copying the resulting byte slice from Bolt. **Unsafe**
```
$ go test -bench BenchmarkRead .
testing: warning: no tests to run
PASS
BenchmarkReadBolt_1024	  500000	      3858 ns/op
BenchmarkReadBolt_10KB	  500000	      3738 ns/op
BenchmarkReadBolt_500KB	 1000000	      3141 ns/op
BenchmarkReadBolt_1MB	 1000000	      3026 ns/op
ok  	github.com/dgraph-io/experiments/db	102.513s
```

Copying the resulting byte slice. **Safe**
```
$ go test -bench BenchmarkRead .
testing: warning: no tests to run
PASS
BenchmarkReadBolt_1024	  200000	      6760 ns/op
BenchmarkReadBolt_10KB	  100000	     21249 ns/op
BenchmarkReadBolt_500KB	   10000	    214449 ns/op
BenchmarkReadBolt_1MB	    3000	    350712 ns/op
ok  	github.com/dgraph-io/experiments/db	80.890s
```

## RocksDB

```
$ go test -bench BenchmarkGet .
PASS
BenchmarkGet_valsize1024	  300000	      5715 ns/op
BenchmarkGet_valsize10KB	   50000	     27619 ns/op
BenchmarkGet_valsize500KB	    2000	    604185 ns/op
BenchmarkGet_valsize1MB	    2000	   1064685 ns/op
ok  	github.com/dgraph-io/dgraph/store	55.029s
```

### Thoughts
DGraph uses append only commit log to sync new mutations to disk before returning.
Every time a posting list gets init, it checks for both the stored posting list and
the mutations committed after the posting list was written. Hence, our access pattern
from store is largely read-only, with fewer writes. This is true, irrespective of how
many writes get commited by the end user.

Hence, BoltDB is a better choice. It performs better for reads/seeks, despite DGraph needing
a value copy. Writes are somewhat slower, but that shouldn't be a problem because of the
above mentioned reasons.
