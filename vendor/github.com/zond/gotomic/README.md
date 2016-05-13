# gotomic

Non blocking data structures for Go.

## Algorithms

The `List` type is implemented using [A Pragmatic Implementation of Non-Blocking Linked-Lists by Timothy L. Harris](http://www.timharris.co.uk/papers/2001-disc.pdf).

The `Hash` type is implemented using [Split-Ordered Lists: Lock-Free Extensible Hash Tables by Ori Shalev and Nir Shavit](http://www.cs.ucf.edu/~dcm/Teaching/COT4810-Spring2011/Literature/SplitOrderedLists.pdf) with the List type used as backend.

The `Transaction` type is implemented using OSTM from [Concurrent Programming Without Locks by Keir Fraser and Tim Harris](http://www.cl.cam.ac.uk/research/srg/netos/papers/2007-cpwl.pdf) with a few tweaks described in https://github.com/zond/gotomic/blob/master/stm.go.

The `Treap` type uses `Transaction` to be non blocking and thread safe, and is based (like all other treaps, I guess) on [Randomized Search Trees by Cecilia Aragon and Raimund Seidel](http://faculty.washington.edu/aragon/pubs/rst89.pdf), but mostly I just used https://github.com/stathat/treap/blob/master/treap.go for reference.

## Performance

On my laptop I created benchmarks for a) regular Go `map` types, b) [Go `map` types protected by `sync.RWMutex`](https://github.com/zond/tools/blob/master/tools.go#L142), c) the `gotomic.Hash`, d) the `gotomic.Treap` type and e) the `github.com/stathat/treap.Tree` type.

The benchmarks for a) and b) can be found at https://github.com/zond/tools/blob/master/tools_test.go#L83, the benchmark for c) at https://github.com/zond/gotomic/blob/master/hash_test.go#L116 and the benchmark for d) and e) at https://github.com/zond/gotomic/blob/master/hash_test.go#L262.

The TL;DR of it all is that the benchmark sets `runtime.GOMAXPROCS` to be `runtime.NumCPU()`, and starts that number of `goroutine`s that just mutates and reads the tested mapping.

Last time I ran these tests I got the following results:

a)

    BenchmarkNativeMap	 5000000	       567 ns/op

b)

    BenchmarkMyMapConc	  200000	     10694 ns/op
    BenchmarkMyMap	 1000000	      1427 ns/op

c)

    BenchmarkHash      500000	      5146 ns/op
    BenchmarkHashConc	  500000	     10599 ns/op

d)

    BenchmarkTreap	   50000	     71250 ns/op
    BenchmarkTreapConc	   10000	    110843 ns/op

e)

    BenchmarkStatHatTreap	 1000000	      4373 ns/op

Also, there are some third party benchmarks available at https://github.com/zond/gotomic/wiki/Benchmarks.

Conclusion: As expected a) is by far the fastest mapping, and it seems that the naive `RWMutex` wrapped native map b) is much faster at single thread operation, and on a weak laptop about as efficient in multi thread operation, compared to c).

However, on more multicored systems (and also a few smaller ones, strangely enough) c) is more efficient than b).

When it comes to the treap class, I am afraid my implementation of STM is really REALLY inefficient. Maybe because I tried to be clever, or because I just botched it someplace. It seems to work, but I reckon that an `RWMutex`-wrapped stathat treap would be preferable in most circumstances.

## Usage

See https://github.com/zond/gotomic/blob/master/examples/example.go or https://github.com/zond/gotomic/blob/master/examples/profile.go

Also, see the tests.

## Documentation

http://go.pkgdoc.org/github.com/zond/gotomic

## Bugs

`Hash` and `List` have no known bugs and seem to work well.

The `Transaction`, `Handle` and `Treap` types are alpha. They seem to work, but are too slow and untrustworthy :/

I have not tried it on more than my personal laptop however, so if you want to try and force it to misbehave on a heftier machine than a 4 cpu MacBook Air please do!

## Improvements

It would be nice to have a Hash#DeleteIfPresent that atomically deletes matching key/value pairs, but since the implementation is slightly harder than trivial and I see no immediate use case I have been too lazy. Tell me if you need it and I might feel motivated :)
