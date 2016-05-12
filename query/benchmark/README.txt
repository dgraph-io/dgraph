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

Benchmarking tests were run for ToJson and ToProtocolBuffer methods. Results
from the `go test` command are tabulated below.

BenchmarkToJson                500       3583970 ns/op      957747 B/op      16115 allocs/op
BenchmarkToProtocolBuffer     1000       2299409 ns/op      566288 B/op       7542 allocs/op

We can see that ToProtocolBuffer method allocates less memory and takes lesser
time than ToJson method.

After changing properties inside a graph.Node from a map to a slice, we can see
further improvements.

BenchmarkToJson                500       3726982 ns/op      957679 B/op      16115 allocs/op
BenchmarkToProtocolBuffer     1000       1954618 ns/op      395603 B/op       7377 allocs/op
