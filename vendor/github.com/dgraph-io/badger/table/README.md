Size of table is 122,173,606 bytes for all benchmarks.

# BenchmarkRead
```
$ go test -bench ^BenchmarkRead$ -run ^$ -count 3
goos: linux
goarch: amd64
pkg: github.com/dgraph-io/badger/table
BenchmarkRead-16    	      10	 153281932 ns/op
BenchmarkRead-16    	      10	 153454443 ns/op
BenchmarkRead-16    	      10	 155349696 ns/op
PASS
ok  	github.com/dgraph-io/badger/table	23.549s
```

Size of table is 122,173,606 bytes, which is ~117MB.

The rate is ~750MB/s using LoadToRAM (when table is in RAM).

To read a 64MB table, this would take ~0.0853s, which is negligible.

# BenchmarkReadAndBuild
```go
$ go test -bench BenchmarkReadAndBuild -run ^$ -count 3
goos: linux
goarch: amd64
pkg: github.com/dgraph-io/badger/table
BenchmarkReadAndBuild-16    	       2	 945041628 ns/op
BenchmarkReadAndBuild-16    	       2	 947120893 ns/op
BenchmarkReadAndBuild-16    	       2	 954909506 ns/op
PASS
ok  	github.com/dgraph-io/badger/table	26.856s
```

The rate is ~122MB/s. To build a 64MB table, this would take ~0.52s. Note that this
does NOT include the flushing of the table to disk. All we are doing above is
reading one table (which is in RAM) and write one table in memory.

The table building takes 0.52-0.0853s ~ 0.4347s.

# BenchmarkReadMerged
Below, we merge 5 tables. The total size remains unchanged at ~122M.

```go
$ go test -bench ReadMerged -run ^$ -count 3
BenchmarkReadMerged-16   	       2	954475788 ns/op
BenchmarkReadMerged-16   	       2	955252462 ns/op
BenchmarkReadMerged-16  	       2	956857353 ns/op
PASS
ok  	github.com/dgraph-io/badger/table	33.327s
```

The rate is ~122MB/s. To read a 64MB table using merge iterator, this would take ~0.52s.

# BenchmarkRandomRead

```go
go test -bench BenchmarkRandomRead$ -run ^$ -count 3
goos: linux
goarch: amd64
pkg: github.com/dgraph-io/badger/table
BenchmarkRandomRead-16    	  300000	      3596 ns/op
BenchmarkRandomRead-16    	  300000	      3621 ns/op
BenchmarkRandomRead-16    	  300000	      3596 ns/op
PASS
ok  	github.com/dgraph-io/badger/table	44.727s
```

For random read benchmarking, we are randomly reading a key and verifying its value.
