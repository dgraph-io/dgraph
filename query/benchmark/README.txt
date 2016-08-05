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

5 August 2016
-------------
These are the benchmarking results after including the Portocol buffer Marshalling
step which was missing in the previous benchmarks and conversion of json byte array to string.
So, this would be the true coparison of Json vs Protocol buffers. 
Also the proto library was changed to gogo protobuf from go protobuf.

BenchmarkToJSON_10_Actor-2                 50000             29559 ns/op            7153 B/op        106 allocs/op
BenchmarkToJSON_10_Director-2              30000             53964 ns/op           13635 B/op        197 allocs/op
BenchmarkToJSON_100_Actor-2                10000            141343 ns/op           38201 B/op        612 allocs/op
BenchmarkToJSON_100_Director-2              5000            318730 ns/op           84097 B/op       1293 allocs/op
BenchmarkToJSON_1000_Actor-2                2000            837214 ns/op          250090 B/op       3855 allocs/op
BenchmarkToJSON_1000_Director-2              500           3703767 ns/op         1029910 B/op      15643 allocs/op
BenchmarkToPB_10_Actor-2                  300000              4093 ns/op             888 B/op         16 allocs/op
BenchmarkToPB_10_Director-2               200000              6173 ns/op            1328 B/op         22 allocs/op
BenchmarkToPB_100_Actor-2                  30000             49327 ns/op           10568 B/op        146 allocs/op
BenchmarkToPB_100_Director-2               20000             69849 ns/op           14768 B/op        219 allocs/op
BenchmarkToPB_1000_Actor-2                  5000            385905 ns/op           72856 B/op        959 allocs/op
BenchmarkToPB_1000_Director-2               2000           1030696 ns/op          207506 B/op       3081 allocs/op
