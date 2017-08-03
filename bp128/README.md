# bp128

Package bp128 implements [SIMD-BP128][1] integer encoding and decoding.
It requires an x86_64/AMD64 CPU that supports SSE2 instructions.

For the original C++ version and other fast encoding and decoding schemes see
[this][2].

[1]: http://arxiv.org/abs/1209.2137
[2]: https://github.com/lemire/SIMDCompressionAndIntersection

## Installation
```sh
go get github.com/robskie/bp128
```

## API Reference

Godoc documentation can be found [here](https://godoc.org/github.com/robskie/bp128).

## Benchmarks

I used a Core i5 2415M (2.3GHz) with 8GB DDR3-1333 RAM for these benchmarks.
For the test data, I generated 2^20 32-bit clustered integers in the range
[0, 2^29) by following [these instructions][3]. These clustered integers are
then grouped into 256KiB chunks before being fed into an encoder/decoder. The
resulting encoding/decoding speed is measured in millions of integers per second
(mis).

You can run these benchmarks by running these commands in terminal.

```bash
cd $GOPATH/src/github.com/robskie/bp128/benchmark
go run benchmark.go
```

[3]: https://github.com/lemire/SIMDCompressionAndIntersection/tree/master/advancedbenchmarking

Here are the results.

```
BenchmarkPack32               894 mis
BenchmarkUnPack32            2279 mis
BenchmarkDeltaPack32         1163 mis
BenchmarkDeltaUnPack32       3443 mis
BenchmarkPack64               511 mis
BenchmarkUnPack64            1596 mis
BenchmarkDeltaPack64          577 mis
BenchmarkDeltaUnPack64       2265 mis
```
