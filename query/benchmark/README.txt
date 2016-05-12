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

BenchmarkToJson                500       3939003 ns/op      957723 B/op      16115 allocs/op
BenchmarkToProtocolBuffer     1000       2288681 ns/op      566287 B/op       7542 allocs/op

We can see that ToProtocolBuffer method allocates less memory and takes lesser
time than ToJson method.