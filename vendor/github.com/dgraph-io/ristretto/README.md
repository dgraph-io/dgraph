# Ristretto
[![Go Doc](https://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/golang-standards/project-layout)
[![Go Report Card](https://img.shields.io/badge/go%20report-A%2B-green)](https://goreportcard.com/report/github.com/dgraph-io/ristretto)
[![Coverage](https://img.shields.io/badge/coverage-79%25-lightgrey)](https://gocover.io/github.com/dgraph-io/ristretto)

Ristretto is a fast, concurrent cache library built with a focus on performance and correctness.

The motivation to build Ristretto comes from the need for a contention-free
cache in [Dgraph][].

[Dgraph]: https://github.com/dgraph-io/dgraph

## Features

* **High Hit Ratios** - with our unique admission/eviction policy pairing, Ristretto's performance is best in class.
	* **Eviction: SampledLFU** - on par with exact LRU and better performance on Search and Database traces.
	* **Admission: TinyLFU** - extra performance with little memory overhead (12 bits per counter).
* **Fast Throughput** - we use a variety of techniques for managing contention and the result is excellent throughput.
* **Cost-Based Eviction** - any large new item deemed valuable can evict multiple smaller items (cost could be anything).
* **Fully Concurrent** - you can use as many goroutines as you want with little throughput degradation. 
* **Metrics** - optional performance metrics for throughput, hit ratios, and other stats.
* **Simple API** - just figure out your ideal `Config` values and you're off and running.

## Status

Ristretto is usable but still under active development. We expect it to be production ready in the near future.

## Table of Contents

* [Usage](#Usage)
	* [Example](#Example)
	* [Config](#Config)
		* [NumCounters](#Config)
		* [MaxCost](#Config)
		* [BufferItems](#Config)
		* [Metrics](#Config)
		* [OnEvict](#Config)
		* [KeyToHash](#Config)
* [Benchmarks](#Benchmarks)
	* [Hit Ratios](#Hit-Ratios)
		* [Search](#Search)
		* [Database](#Database)
		* [Looping](#Looping)
		* [CODASYL](#CODASYL)
	* [Throughput](#Throughput)
		* [Mixed](#Mixed)
		* [Read](#Read)
		* [Write](#Write)
* [FAQ](#FAQ)

## Usage

### Example

```go
func main() {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		panic(err)
	}

	// set a value with a cost of 1
	cache.Set("key", "value", 1)
	
	// wait for value to pass through buffers
	time.Sleep(10 * time.Millisecond)

	value, found := cache.Get("key")
	if !found {
		panic("missing value")
	}
	fmt.Println(value)
	cache.Del("key")
}
```

### Config

The `Config` struct is passed to `NewCache` when creating Ristretto instances (see the example above). 

**NumCounters** `int64`

NumCounters is the number of 4-bit access counters to keep for admission and eviction. We've seen good performance in setting this to 10x the number of items you expect to keep in the cache when full. 

For example, if you expect each item to have a cost of 1 and MaxCost is 100, set NumCounters to 1,000. Or, if you use variable cost values but expect the cache to hold around 10,000 items when full, set NumCounters to 100,000. The important thing is the *number of unique items* in the full cache, not necessarily the MaxCost value. 

**MaxCost** `int64`

MaxCost is how eviction decisions are made. For example, if MaxCost is 100 and a new item with a cost of 1 increases total cache cost to 101, 1 item will be evicted. 

MaxCost can also be used to denote the max size in bytes. For example, if MaxCost is 1,000,000 (1MB) and the cache is full with 1,000 1KB items, a new item (that's accepted) would cause 5 1KB items to be evicted. 

MaxCost could be anything as long as it matches how you're using the cost values when calling Set. 

**BufferItems** `int64`

BufferItems is the size of the Get buffers. The best value we've found for this is 64. 

If for some reason you see Get performance decreasing with lots of contention (you shouldn't), try increasing this value in increments of 64. This is a fine-tuning mechanism and you probably won't have to touch this.

**Metrics** `bool`

Metrics is true when you want real-time logging of a variety of stats. The reason this is a Config flag is because there's a 10% throughput performance overhead. 

**OnEvict** `func(keyHash uint64, value interface{}, cost int64)`

OnEvict is called for every eviction.

**KeyToHash** `func(key interface{}) uint64`

KeyToHash is the hashing algorithm used for every key. If this is nil, Ristretto has a variety of [defaults depending on the underlying interface type](https://github.com/dgraph-io/ristretto/blob/master/z/z.go#L19-L41).

## Benchmarks

The benchmarks can be found in https://github.com/dgraph-io/benchmarks/tree/master/cachebench/ristretto.

### Hit Ratios

#### Search

This trace is described as "disk read accesses initiated by a large commercial
search engine in response to various web search requests."

<p align="center">
	<img src="https://raw.githubusercontent.com/karlmcguire/karlmcguire.com/master/docs/Hit%20Ratios%20-%20Search%20(ARC-S3).svg?sanitize=true">
</p>

#### Database

This trace is described as "a database server running at a commercial site
running an ERP application on top of a commercial database."

<p align="center">
	<img src="https://raw.githubusercontent.com/karlmcguire/karlmcguire.com/master/docs/Hit%20Ratios%20-%20Database%20(ARC-DS1).svg?sanitize=true">
</p>

#### Looping

This trace demonstrates a looping access pattern.

<p align="center">
	<img src="https://raw.githubusercontent.com/karlmcguire/karlmcguire.com/master/docs/Hit%20Ratios%20-%20Glimpse%20(LIRS-GLI).svg?sanitize=true">
</p>

#### CODASYL

This trace is described as "references to a CODASYL database for a one hour
period."

<p align="center">
	<img src="https://raw.githubusercontent.com/karlmcguire/karlmcguire.com/master/docs/Hit%20Ratios%20-%20CODASYL%20(ARC-OLTP).svg?sanitize=true">
</p>

### Throughput

All throughput benchmarks were ran on an Intel Core i7-8700K (3.7GHz) with 16gb
of RAM.

#### Mixed

<p align="center">
	<img src="https://raw.githubusercontent.com/karlmcguire/karlmcguire.com/master/docs/Throughput%20-%20Mixed.svg?sanitize=true">
</p>

#### Read

<p align="center">
	<img src="https://raw.githubusercontent.com/karlmcguire/karlmcguire.com/master/docs/Throughput%20-%20Read%20(Zipfian).svg?sanitize=true">
</p>

#### Write

<p align="center">
	<img src="https://raw.githubusercontent.com/karlmcguire/karlmcguire.com/master/docs/Throughput%20-%20Write%20(Zipfian).svg?sanitize=true">
</p>

## FAQ

### How are you achieving this performance? What shortcuts are you taking?

We go into detail in the [Ristretto blog post](https://blog.dgraph.io/post/introducing-ristretto-high-perf-go-cache/), but in short: our throughput performance can be attributed to a mix of batching and eventual consistency. Our hit ratio performance is mostly due to an excellent [admission policy](https://arxiv.org/abs/1512.00727) and SampledLFU eviction policy.

As for "shortcuts," the only thing Ristretto does that could be construed as one is dropping some Set calls. That means a Set call for a new item (updates are guaranteed) isn't guaranteed to make it into the cache. The new item could be dropped at two points: when passing through the Set buffer or when passing through the admission policy. However, this doesn't affect hit ratios much at all as we expect the most popular items to be Set multiple times and eventually make it in the cache. 

### Is Ristretto distributed?

No, it's just like any other Go library that you can import into your project and use in a single process. 
