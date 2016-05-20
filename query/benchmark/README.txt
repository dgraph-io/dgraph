The files in this folder contain gobencoded data for a processed SubGraph for
the following queries. The number at the end(10,100,1000) of the files 
represents the number of entities in the results of the query.

Actors query
{
        me(_xid_:m.08624h) {
         type.object.name.en
         film.actor.film {
             film.performance.film {
                 type.object.name.en
             }
         }
    }
}

Directors query
{
        me(_xid_:m.05dxl_) {
          type.object.name.en
          film.director.film  {
                film.film.genre {
                  type.object.name.en
                }
          }
    }
}

14 May 2016
Benchmarking tests were run for ToJson and ToProtocolBuffer methods. Results
from the `go test` command are tabulated below.

BenchmarkToJSON_10_Actor         20000       92797 ns/op     22616 B/op      319 allocs/op
BenchmarkToJSON_10_Director      20000       87246 ns/op     21111 B/op      303 allocs/op
BenchmarkToJSON_100_Actor         2000      774767 ns/op    207893 B/op     2670 allocs/op
BenchmarkToJSON_100_Director      2000      579467 ns/op    142811 B/op     2103 allocs/op
BenchmarkToJSON_1000_Actor         200     7903001 ns/op   1904863 B/op    24712 allocs/op
BenchmarkToJSON_1000_Director      300     4335375 ns/op    957728 B/op    16115 allocs/op
BenchmarkToPB_10_Actor          100000       19672 ns/op      3176 B/op       60 allocs/op
BenchmarkToPB_10_Director       100000       17891 ns/op      3096 B/op       60 allocs/op
BenchmarkToPB_100_Actor          10000      372288 ns/op     30728 B/op      556 allocs/op
BenchmarkToPB_100_Director        5000      221506 ns/op     37272 B/op      701 allocs/op
BenchmarkToPB_1000_Actor           500     2612757 ns/op    296486 B/op     5383 allocs/op
BenchmarkToPB_1000_Director        300     3980677 ns/op    395600 B/op     7376 allocs/op

We can see that ToProtocolBuffer method allocates less memory and takes lesser
time than ToJson method.

20 May 2016
These are the benchmarking results after changing type of Value in x.DirectedEdge,
type of ObjectValue in NQuad to []byte and using the byte slice directly from
flatbuffers instead of parsing into an interface.(Commit SHA - 480b1337f). We
can see tremendous improvement(>50% on an average) for all metrics. For exact
percentage change, we can run benchcmp to compare the metrics.

BenchmarkToJSON_10_Actor-4         50000       27497 ns/op      7626 B/op      113 allocs/op
BenchmarkToJSON_10_Director-4      30000       54688 ns/op     16229 B/op      228 allocs/op
BenchmarkToJSON_100_Actor-4        10000      137853 ns/op     37333 B/op      619 allocs/op
BenchmarkToJSON_100_Director-4      5000      334310 ns/op     92971 B/op     1428 allocs/op
BenchmarkToJSON_1000_Actor-4        2000      780858 ns/op    240419 B/op     3863 allocs/op
BenchmarkToJSON_1000_Director-4      500     3599711 ns/op    986670 B/op    15867 allocs/op
BenchmarkToPB_10_Actor-4          500000        3252 ns/op       664 B/op       15 allocs/op
BenchmarkToPB_10_Director-4       300000        4991 ns/op       976 B/op       21 allocs/op
BenchmarkToPB_100_Actor-4          30000       44771 ns/op      8008 B/op      145 allocs/op
BenchmarkToPB_100_Director-4       20000       60952 ns/op     10672 B/op      218 allocs/op
BenchmarkToPB_1000_Actor-4          5000      366134 ns/op     56217 B/op      958 allocs/op
BenchmarkToPB_1000_Director-4       2000      908611 ns/op    150163 B/op     3080 allocs/op
