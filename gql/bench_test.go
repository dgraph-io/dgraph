package gql

import (
	"testing"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/stretchr/testify/require"
)

var q1 = `{
  debug(allof("type.object.name.en", "steven spielberg")) {
    type.object.name.en
    film.director.film {
      type.object.name.en
      film.film.initial_release_date
      film.film.country
      film.film.starring {
        film.performance.actor {
          type.object.name.en
        }
        film.performance.character {
          type.object.name.en
        }
      }
      film.film.genre {
        type.object.name.en
      }
    }
  }
}`

var q2 = `{
  debug(anyof("type.object.name.en","big lebowski")) {
    type.object.name.en
    film.film.initial_release_date
    film.film.country
    film.film.starring {
      film.performance.actor {
        type.object.name.en
      }
      film.performance.character {
        type.object.name.en
      }
    }
    film.film.genre {
      type.object.name.en
    }
  }
}`

var q3 = `{
  debug(_xid_: m.06pj8) {
    type.object.name.en
    film.director.film @filter(allof("type.object.name.en", "jones indiana") || allof("type.object.name.en", "jurassic park"))  {
	      _uid_
	      type.object.name.en
     }
  }
}`

var q4 = `{
  debug(_xid_: m.0bxtg) {
    type.object.name.en
    film.director.film @filter(geq("film.film.initial_release_date", "1970-01-01")) {
      film.film.initial_release_date
      type.object.name.en
    }
  }
}`

var q5 = `{
   debug(allof("type.object.name.en", "steven spielberg")) {
     type.object.name.en
     film.director.film(order: film.film.initial_release_date) {
       type.object.name.en
       film.film.initial_release_date
     }
   }
}`

var m1 = `mutation {
 set {
   _:class <student> _:x .
   _:class <name> "awesome class" .
   _:x <name> "alice" .
   _:x <planet> "Mars" .
   _:x <friend> _:y .
   _:y <name> "bob" .
 }
}`

var sc = `scalar type.object.name.en: string @index
scalar film.film.initial_release_date: date @index`

func benchmarkParsingHelper(b *testing.B, q string) {
	schema.ParseBytes([]byte(sc))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Parse(q)
		require.NoError(b, err)
	}
}

func benchmarkParsingParallelHelper(b *testing.B, q string) {
	schema.ParseBytes([]byte(sc))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := Parse(q)
			require.NoError(b, err)
		}
	})
}

func Benchmark_directors(b *testing.B)          { benchmarkParsingHelper(b, q1) }
func Benchmark_Movies(b *testing.B)             { benchmarkParsingHelper(b, q2) }
func Benchmark_Filters(b *testing.B)            { benchmarkParsingHelper(b, q3) }
func Benchmark_Geq(b *testing.B)                { benchmarkParsingHelper(b, q4) }
func Benchmark_Date(b *testing.B)               { benchmarkParsingHelper(b, q5) }
func Benchmark_Mutation(b *testing.B)           { benchmarkParsingHelper(b, m1) }
func Benchmark_directors_parallel(b *testing.B) { benchmarkParsingParallelHelper(b, q1) }
func Benchmark_Movies_parallel(b *testing.B)    { benchmarkParsingParallelHelper(b, q2) }
func Benchmark_Filters_parallel(b *testing.B)   { benchmarkParsingParallelHelper(b, q3) }
func Benchmark_Geq_parallel(b *testing.B)       { benchmarkParsingParallelHelper(b, q4) }
func Benchmark_Date_parallel(b *testing.B)      { benchmarkParsingParallelHelper(b, q5) }
func Benchmark_Mutation_parallel(b *testing.B)  { benchmarkParsingParallelHelper(b, m1) }
