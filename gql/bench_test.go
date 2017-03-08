package gql

import (
	"testing"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/stretchr/testify/require"
)

var sc = `scalar type.object.name.en: string @index
scalar film.film.initial_release_date: date @index`

func benchmarkParsingHelper(b *testing.B, q string) {
	schema.ParseBytes([]byte(sc))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Parse(q)
		require.NoError(b, err)
	}
}

func benchmarkParsingParallelHelper(b *testing.B, q string) {
	schema.ParseBytes([]byte(sc))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := Parse(q)
			require.NoError(b, err)
		}
	})
}

func Benchmark_directors(b *testing.B)             { benchmarkParsingHelper(b, q1) }
func Benchmark_Movies(b *testing.B)                { benchmarkParsingHelper(b, q2) }
func Benchmark_Filters(b *testing.B)               { benchmarkParsingHelper(b, q3) }
func Benchmark_Geq(b *testing.B)                   { benchmarkParsingHelper(b, q4) }
func Benchmark_Date(b *testing.B)                  { benchmarkParsingHelper(b, q5) }
func Benchmark_Mutation(b *testing.B)              { benchmarkParsingHelper(b, m1) }
func Benchmark_Mutation1000(b *testing.B)          { benchmarkParsingHelper(b, m1000) }
func Benchmark_directors_parallel(b *testing.B)    { benchmarkParsingParallelHelper(b, q1) }
func Benchmark_Movies_parallel(b *testing.B)       { benchmarkParsingParallelHelper(b, q2) }
func Benchmark_Filters_parallel(b *testing.B)      { benchmarkParsingParallelHelper(b, q3) }
func Benchmark_Geq_parallel(b *testing.B)          { benchmarkParsingParallelHelper(b, q4) }
func Benchmark_Date_parallel(b *testing.B)         { benchmarkParsingParallelHelper(b, q5) }
func Benchmark_Mutation_parallel(b *testing.B)     { benchmarkParsingParallelHelper(b, m1) }
func Benchmark_Mutation1000_parallel(b *testing.B) { benchmarkParsingParallelHelper(b, m1000) }

var q1 = `{
  debug(allofterms("type.object.name.en", "steven spielberg")) {
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
  debug(anyofterms("type.object.name.en","big lebowski")) {
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
  debug(id: m.06pj8) {
    type.object.name.en
    film.director.film @filter(allofterms("type.object.name.en", "jones indiana") or allofterms("type.object.name.en", "jurassic park"))  {
	      _uid_
	      type.object.name.en
     }
  }
}`

var q4 = `{
  debug(id: m.0bxtg) {
    type.object.name.en
    film.director.film @filter(geq("film.film.initial_release_date", "1970-01-01")) {
      film.film.initial_release_date
      type.object.name.en
    }
  }
}`

var q5 = `{
   debug(allofterms("type.object.name.en", "steven spielberg")) {
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

var m1000 = `mutation {
	set {
		<film.director.film>	<http://www.w3.org/2000/01/rdf-schema#domain>	<film.director>	.
<film.director.film>	<http://www.w3.org/2000/01/rdf-schema#range>	<film.film>	.
<film.director.film>	<type.object.name>	"Films directed"@en	.
<film.director.film>	<type.property.expected_type>	<film.film>	.
<film.director.film>	<type.property.schema>	<film.director>	.
<film.film.directed_by>	<http://www.w3.org/2000/01/rdf-schema#domain>	<film.film>	.
<film.film.directed_by>	<http://www.w3.org/2000/01/rdf-schema#range>	<film.director>	.
<film.film.directed_by>	<http://www.w3.org/2002/07/owl#inverseOf>	<film.director.film>	.
<film.film.directed_by>	<type.object.name>	"Directed by"@en	.
<film.film.directed_by>	<type.property.expected_type>	<film.director>	.
<film.film.directed_by>	<type.property.reverse_property>	<film.director.film>	.
<film.film.directed_by>	<type.property.schema>	<film.film>	.
<film.film.initial_release_date>	<http://www.w3.org/2000/01/rdf-schema#domain>	<film.film>	.
<film.film.initial_release_date>	<type.object.name>	"Initial release date"@en	.
<film.film.initial_release_date>	<type.property.schema>	<film.film>	.
<g.112yf7mpn>	<film.film.directed_by>	<m.0g7x9yx>	.
<g.112yf7mpn>	<film.film.initial_release_date>	"1976-06-19"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112yf7mpn>	<type.object.name>	"Jail Breakers"@en	.
<g.112yf7mpn>	<type.object.name>	"脱走遊戯"@ja	.
<g.112yfbfms>	<film.film.directed_by>	<m.0gdqy>	.
<g.112yfbfms>	<film.film.initial_release_date>	"1986"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfbfms>	<type.object.name>	"Grandeur et decadence dun petit commerce de Cinema"@en	.
<g.112yfcm2y>	<film.film.directed_by>	<m.021r60>	.
<g.112yfcm2y>	<film.film.directed_by>	<m.0bggv2d>	.
<g.112yfcm2y>	<film.film.initial_release_date>	"1991"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfcm2y>	<type.object.name>	"The Doors: Live in Europe (1968)"@en	.
<g.112yfcm7n>	<film.film.directed_by>	<m.0n1y549>	.
<g.112yfcm7n>	<film.film.initial_release_date>	"2003"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfcm7n>	<type.object.name>	"Smert Tairova"@en	.
<g.112yfd67k>	<film.director.film>	<m.0gwz00g>	.
<g.112yfd67k>	<type.object.name>	"Joe Pearson"@en	.
<g.112yffjt5>	<film.film.directed_by>	<m.0cqkn>	.
<g.112yffjt5>	<type.object.name>	"The Devilish Plank"@en	.
<g.112yfh1yt>	<film.film.directed_by>	<m.03cnlm6>	.
<g.112yfh1yt>	<film.film.initial_release_date>	"1973"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfh1yt>	<type.object.name>	"Op de Hollandse Toer"@en	.
<g.112yfj_2x>	<film.film.directed_by>	<m.0fqqkcp>	.
<g.112yfj_2x>	<film.film.initial_release_date>	"1975-03-08"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112yfj_2x>	<type.object.name>	"Mamushi to aodaishô"@en	.
<g.112yfj_2x>	<type.object.name>	"まむしと青大将"@ja	.
<g.112yfj59p>	<film.film.directed_by>	<m.0cqkn>	.
<g.112yfj59p>	<film.film.initial_release_date>	"1909"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfj59p>	<type.object.name>	"Le papillon fantastique"@en	.
<g.112yfjvnt>	<film.film.directed_by>	<m.02pqr1r>	.
<g.112yfjvnt>	<film.film.initial_release_date>	"1967-12-23"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112yfjvnt>	<type.object.name>	"Abashiri bangaichi: Fubuki no tôsô"@en	.
<g.112yfjvnt>	<type.object.name>	"網走番外地 吹雪の斗争"@ja	.
<g.112yfk5g_>	<film.film.directed_by>	<m.0cm_67>	.
<g.112yfk5g_>	<film.film.initial_release_date>	"1982"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfk5g_>	<type.object.name>	"The imperilled"@en	.
<g.112yfkr90>	<film.film.directed_by>	<m.0cn6lr>	.
<g.112yfkr90>	<film.film.initial_release_date>	"1964"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfkr90>	<type.object.name>	"The Newspaper Seller"@en	.
<g.112yflv95>	<film.film.directed_by>	<m.0fm1fy>	.
<g.112yflv95>	<film.film.initial_release_date>	"1999"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yflv95>	<type.object.name>	"My Father, My Mother, My Brothers and My Sisters"@en	.
<g.112yflxpr>	<film.film.directed_by>	<m.0c68p2j>	.
<g.112yflxpr>	<film.film.initial_release_date>	"2011-08-06"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112yflxpr>	<type.object.name>	"Tahrir: Liberation Square"@en	.
<g.112yfmgyq>	<film.film.directed_by>	<g.1238s68y>	.
<g.112yfmgyq>	<film.film.directed_by>	<m.0fpqwmd>	.
<g.112yfmgyq>	<film.film.initial_release_date>	"1945"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfmgyq>	<type.object.name>	"Father Serge"@en	.
<g.112yfndvf>	<film.film.directed_by>	<m.0gm5yd1>	.
<g.112yfndvf>	<film.film.initial_release_date>	"2012-03-11"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112yfndvf>	<type.object.name>	"Father's Lullaby"@en	.
<g.112yfndvf>	<type.object.name>	"手機裡的眼淚"@zh-Hant	.
<g.112yfpbts>	<film.director.film>	<m.0j6568c>	.
<g.112yfpbts>	<type.object.name>	"Ali Benjelloun"@en	.
<g.112yfpm2y>	<film.film.directed_by>	<m.0jy55n>	.
<g.112yfpm2y>	<film.film.initial_release_date>	"2001"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfpm2y>	<type.object.name>	"Classic Albums – Elton John: Goodbye Yellow Brick Road"@en	.
<g.112yfq9px>	<film.film.directed_by>	<m.0l6wj>	.
<g.112yfq9px>	<film.film.initial_release_date>	"1925"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfq9px>	<type.object.name>	"What Price Goofy"@en	.
<g.112yfqg53>	<film.film.directed_by>	<m.0s9p9kw>	.
<g.112yfqg53>	<film.film.initial_release_date>	"1969"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfqg53>	<type.object.name>	"The Prostitute"@en	.
<g.112yfqr1b>	<film.film.directed_by>	<m.0gvdh8z>	.
<g.112yfqr1b>	<film.film.initial_release_date>	"1927-10-26"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112yfqr1b>	<type.object.name>	"At Daybreak"@en	.
<g.112yfsbd4>	<film.film.directed_by>	<m.04tm5c>	.
<g.112yfsbd4>	<film.film.initial_release_date>	"1989-07-08"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112yfsbd4>	<type.object.name>	"24 Hour Playboy"@en	.
<g.112yftzpc>	<film.film.directed_by>	<m.0cn6lr>	.
<g.112yftzpc>	<film.film.initial_release_date>	"1963"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yftzpc>	<type.object.name>	"Chafika el Keptia"@en	.
<g.112yfykfg>	<film.film.directed_by>	<m.0r4hnv_>	.
<g.112yfykfg>	<film.film.initial_release_date>	"1970"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yfykfg>	<type.object.name>	"How the Cossacks Played Football"@en	.
<g.112yg0jyk>	<film.film.directed_by>	<m.04jmbv8>	.
<g.112yg0jyk>	<film.film.initial_release_date>	"2011"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yg0jyk>	<type.object.name>	"Memoria de mis putas tristes"@en	.
<g.112yg35mc>	<film.film.directed_by>	<m.0dm3khz>	.
<g.112yg35mc>	<film.film.initial_release_date>	"1984"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yg35mc>	<type.object.name>	"Hocus Pocus"@en	.
<g.112yg5flb>	<film.film.directed_by>	<m.0db2f0l>	.
<g.112yg5flb>	<film.film.initial_release_date>	"2008"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112yg5flb>	<type.object.name>	"The Joy of Singing"@en	.
<g.112yg66ms>	<film.director.film>	<m.0gzlx2x>	.
<g.112yg66ms>	<type.object.name>	"Marco Mattolini"@en	.
<g.112yg6zh9>	<film.film.directed_by>	<m.0b_z9ym>	.
<g.112yg6zh9>	<film.film.initial_release_date>	"2005-12-13"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112yg6zh9>	<type.object.name>	"Partner(s)"@en	.
<g.112yg7qnq>	<film.film.directed_by>	<m.0x1_hrv>	.
<g.112yg7qnq>	<film.film.directed_by>	<m.0x1_k2d>	.
<g.112yg7qnq>	<film.film.initial_release_date>	"2012-08-17"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112yg7qnq>	<type.object.name>	"The Fierce Wife Final Episode"@en	.
<g.112yg7qnq>	<type.object.name>	"結婚って、幸せですか THE MOVIE"@ja	.
<g.112yg8hjj>	<film.director.film>	<m.0kfb6q7>	.
<g.112yg8hjj>	<type.object.name>	"Diana Naecke"@en	.
<g.112ygcc01>	<film.director.film>	<m.0gm3l8s>	.
<g.112ygcc01>	<type.object.name>	"Theodora Remundová"@en	.
<g.112ygczst>	<film.director.film>	<m.0d54r8d>	.
<g.112ygczst>	<film.director.film>	<m.0fq63bx>	.
<g.112ygczst>	<type.object.name>	"Hans H. König"@en	.
<g.112ygf_gh>	<film.film.directed_by>	<m.0b_zm3r>	.
<g.112ygf_gh>	<film.film.directed_by>	<m.0cfrmn>	.
<g.112ygf_gh>	<film.film.initial_release_date>	"1991-04-24"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.112ygf_gh>	<type.object.name>	"Les secrets professionnels du Dr Apfelglück"@en	.
<g.112ygg67m>	<film.film.directed_by>	<m.0fq6czh>	.
<g.112ygg67m>	<film.film.initial_release_date>	"2010"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.112ygg67m>	<type.object.name>	"Rolling With Stone"@en	.
<g.113qb9gcx>	<film.director.film>	<m.012hw58n>	.
<g.113qb9gcx>	<type.object.name>	"Mathias Poledna"@en	.
<g.113qbb2ck>	<film.director.film>	<m.0vx63rn>	.
<g.113qbb2ck>	<film.director.film>	<m.0vx65nw>	.
<g.113qbb2ck>	<film.director.film>	<m.0vx69qx>	.
<g.113qbb2ck>	<type.object.name>	"Ettore Buchi"@en	.
<g.113qbbykk>	<film.film.directed_by>	<m.0j5yjtj>	.
<g.113qbbykk>	<film.film.initial_release_date>	"2012-02-16"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.113qbbykk>	<type.object.name>	"Joyful Reunion"@en	.
<g.113qbbykk>	<type.object.name>	"飲食男女：好遠又好近"@zh-Hant	.
<g.113qbfj4t>	<film.film.directed_by>	<m.0kwrss4>	.
<g.113qbfj4t>	<film.film.initial_release_date>	"1979"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.113qbfj4t>	<type.object.name>	"Soldiers Never Cry"@en	.
<g.113qbgbl2>	<film.film.directed_by>	<m.02rjhym>	.
<g.113qbgbl2>	<film.film.directed_by>	<m.04ypjwb>	.
<g.113qbgbl2>	<film.film.initial_release_date>	"2010-12-25"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.113qbgbl2>	<type.object.name>	"Scrat's Continental Crack-up"@en	.
<g.113qbgpmy>	<film.film.initial_release_date>	"1993-06-16"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.113qbgpmy>	<type.object.name>	"L'Enfant Lion"@en	.
<g.113qbhd9g>	<film.film.directed_by>	<m.0nh4gv0>	.
<g.113qbhd9g>	<film.film.initial_release_date>	"2012-07-07"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.113qbhd9g>	<type.object.name>	"Soup: umare kawari no monogatari"@en	.
<g.113qbhd9g>	<type.object.name>	"スープ〜生まれ変わりの物語〜"@ja	.
<g.113qbkxv5>	<film.film.directed_by>	<m.0b_qz9k>	.
<g.113qbkxv5>	<film.film.initial_release_date>	"2012-02-01"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.113qbkxv5>	<type.object.name>	"Golden Cage"@en	.
<g.113qbmjzn>	<film.film.directed_by>	<m.0114zknz>	.
<g.113qbmjzn>	<film.film.initial_release_date>	"2012-10"^^<http://www.w3.org/2001/XMLSchema#gYearMonth>	.
<g.113qbmjzn>	<type.object.name>	"Eastalgia"@en	.
<g.113qbq6n6>	<film.director.film>	<m.0gy7yhx>	.
<g.113qbq6n6>	<type.object.name>	"Jens Becker"@en	.
<g.113qbtz3_>	<film.director.film>	<m.0x1v0sp>	.
<g.113qbtz3_>	<type.object.name>	"Nasser Mehdipoor"@en	.
<g.113tzbvc4>	<film.director.film>	<m.0gy7_sj>	.
<g.113tzbvc4>	<film.director.film>	<m.0gy7x3l>	.
<g.113tzbvc4>	<type.object.name>	"Pasquale Misuraca"@en	.
<g.119pf_dmj>	<film.director.film>	<g.1q6g0dlgp>	.
<g.119pf_dmj>	<film.director.film>	<m.0cryk_3>	.
<g.119pf_dmj>	<film.director.film>	<m.0gzlpmc>	.
<g.119pf_dmj>	<film.director.film>	<m.0vx_wlm>	.
<g.119pf_dmj>	<type.object.name>	"Andras Solyom"@en	.
<g.119pflycz>	<film.director.film>	<m.0n43trd>	.
<g.119pflycz>	<type.object.name>	"Ji-u Hong"@en	.
<g.119pft2q0>	<film.film.directed_by>	<m.0fq86dx>	.
<g.119pft2q0>	<film.film.initial_release_date>	"1968-11-02"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.119pft2q0>	<type.object.name>	"コント55号 世紀の大弱点"@ja	.
<g.119pft2q0>	<type.object.name>	"Konto gojugo-go: Seiki no daijukuten"@en	.
<g.119pftqkr>	<film.film.directed_by>	<m.0bfmvzc>	.
<g.119pftqkr>	<film.film.initial_release_date>	"1996"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.119pftqkr>	<type.object.name>	"Reprise"@en	.
<g.119pfys8q>	<film.film.directed_by>	<m.0flddp>	.
<g.119pfys8q>	<film.film.initial_release_date>	"1917-04-29"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.119pfys8q>	<type.object.name>	"Sunshine and Gold"@en	.
<g.119pg2bd6>	<film.film.directed_by>	<m.0fd_k7>	.
<g.119pg2bd6>	<film.film.initial_release_date>	"1997"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.119pg2bd6>	<type.object.name>	"Moon Over Tao"@en	.
<g.119pg2bd6>	<type.object.name>	"タオの月"@ja	.
<g.119pg3fjm>	<film.director.film>	<m.0gm7900>	.
<g.119pg3fjm>	<type.object.name>	"Pasquale Marino"@en	.
<g.119pg9nyr>	<film.film.directed_by>	<m.0cm_67>	.
<g.119pg9nyr>	<film.film.initial_release_date>	"1977"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.119pg9nyr>	<type.object.name>	"Do kalle-shagh"@en	.
<g.119pgcx6v>	<film.film.directed_by>	<m.04m_083>	.
<g.119pgcx6v>	<film.film.initial_release_date>	"1978"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.119pgcx6v>	<type.object.name>	"Dirty Dreamer"@en	.
<g.119pgddwp>	<film.film.directed_by>	<m.0331xx>	.
<g.119pgddwp>	<film.film.initial_release_date>	"1935"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.119pgddwp>	<type.object.name>	"The Fashion Side of Hollywood"@en	.
<g.119pgptg8>	<film.director.film>	<m.0crwqyx>	.
<g.119pgptg8>	<type.object.name>	"Irina Poplavskaya"@en	.
<g.119pgyn8d>	<film.film.directed_by>	<m.0ndbk69>	.
<g.119pgyn8d>	<film.film.initial_release_date>	"1998"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.119pgyn8d>	<type.object.name>	"Killer of Small Fishes"@en	.
<g.119x6_hb5>	<film.film.directed_by>	<g.1211j7bl>	.
<g.119x6_hb5>	<film.film.initial_release_date>	"2011"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.119x6_hb5>	<type.object.name>	"Yes We Can!"@en	.
<g.119x72t9z>	<film.film.directed_by>	<m.0vsnglg>	.
<g.119x72t9z>	<film.film.initial_release_date>	"1978"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.119x72t9z>	<type.object.name>	"Melodies, Melodies..."@en	.
<g.119x73kjh>	<film.film.directed_by>	<m.03m5x1g>	.
<g.119x73kjh>	<film.film.initial_release_date>	"1940-06-12"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.119x73kjh>	<type.object.name>	"Only the Valiant"@en	.
<g.119x75q80>	<film.film.directed_by>	<m.0cm4myy>	.
<g.119x75q80>	<film.film.initial_release_date>	"2009-05-16"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.119x75q80>	<type.object.name>	"Socarrat"@en	.
<g.119x7711l>	<film.film.directed_by>	<m.0byp7y0>	.
<g.119x7711l>	<film.film.initial_release_date>	"1988"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.119x7711l>	<type.object.name>	"The Donkey Who Drank the Moon"@en	.
<g.119x77m2p>	<film.film.directed_by>	<m.0bysf0k>	.
<g.119x77m2p>	<film.film.initial_release_date>	"2012-03-03"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.119x77m2p>	<type.object.name>	"Tanemaku tabibito: Minori no cha"@en	.
<g.119x77m2p>	<type.object.name>	"種まく旅人～みのりの茶～"@ja	.
<g.11b419wth>	<film.film.directed_by>	<m.0k6q2_>	.
<g.11b419wth>	<film.film.initial_release_date>	"2012-10-31"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b419wth>	<type.object.name>	"The Crossing"@en	.
<g.11b419z8x>	<film.film.directed_by>	<m.0g_qsyz>	.
<g.11b419z8x>	<film.film.initial_release_date>	"1949-11-29"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b419z8x>	<type.object.name>	"Breaking the Wall"@en	.
<g.11b419zwv>	<film.film.directed_by>	<m.0gyccp9>	.
<g.11b419zwv>	<film.film.initial_release_date>	"1986"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b419zwv>	<type.object.name>	"Tommaso Blu"@en	.
<g.11b41bmz_>	<film.film.directed_by>	<m.0bypchh>	.
<g.11b41bmz_>	<film.film.initial_release_date>	"1983-08-27"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b41bmz_>	<type.object.name>	"Pine Tree"@en	.
<g.11b41bs97>	<film.film.directed_by>	<m.0fhbl_>	.
<g.11b41bs97>	<film.film.initial_release_date>	"1957-09-18"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b41bs97>	<type.object.name>	"Lost Youth"@en	.
<g.11b41cl0v>	<film.film.initial_release_date>	"1949-11-15"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b41cl0v>	<type.object.name>	"Pasi"@en	.
<g.11b41dh4z>	<film.film.directed_by>	<m.03nt0_k>	.
<g.11b41dh4z>	<film.film.directed_by>	<m.0g5j8hm>	.
<g.11b41dh4z>	<film.film.initial_release_date>	"2000"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b41dh4z>	<type.object.name>	"Fleeing by Night"@en	.
<g.11b41hbyc>	<film.film.directed_by>	<m.010gdx68>	.
<g.11b41hbyc>	<film.film.directed_by>	<m.010gdx7g>	.
<g.11b41hbyc>	<film.film.initial_release_date>	"2012"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b41hbyc>	<type.object.name>	"Thomás Tristonho"@en	.
<g.11b41jfzq>	<film.film.directed_by>	<m.0cqkn>	.
<g.11b41jfzq>	<type.object.name>	"Tit for Tat"@en	.
<g.11b41l_3m>	<film.film.directed_by>	<m.0cn6lr>	.
<g.11b41l_3m>	<film.film.initial_release_date>	"1975-12-21"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b41l_3m>	<type.object.name>	"Badiaa Masabny"@en	.
<g.11b41nzhq>	<film.film.directed_by>	<m.0gtlngb>	.
<g.11b41nzhq>	<film.film.initial_release_date>	"1966-06-10"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b41nzhq>	<type.object.name>	"Early Rain"@en	.
<g.11b5lzm6b0>	<film.film.directed_by>	<m.0273n9d>	.
<g.11b5lzm6b0>	<film.film.initial_release_date>	"2008"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b5lzm6b0>	<type.object.name>	"The True Story of the Three Little Pigs"@en	.
<g.11b5m04ftm>	<film.film.directed_by>	<m.0gtmp7d>	.
<g.11b5m04ftm>	<film.film.initial_release_date>	"1962-01-19"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b5m04ftm>	<type.object.name>	"When the Cloud Scatters Away"@en	.
<g.11b5m18cr9>	<film.director.film>	<m.01132cl9>	.
<g.11b5m18cr9>	<type.object.name>	"Joana Nin"@en	.
<g.11b5m1rphr>	<film.film.directed_by>	<m.0gwkcyb>	.
<g.11b5m1rphr>	<film.film.initial_release_date>	"1966-07-16"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b5m1rphr>	<type.object.name>	"A Tearstained Crown"@en	.
<g.11b5m1xljn>	<film.director.film>	<m.010sjp25>	.
<g.11b5m1xljn>	<type.object.name>	"Yael Reuveny"@en	.
<g.11b5m2729c>	<film.film.directed_by>	<m.0g5hr3q>	.
<g.11b5m2729c>	<film.film.initial_release_date>	"1969-09-04"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b5m2729c>	<type.object.name>	"A Returned Singer"@en	.
<g.11b5m2ph7y>	<film.film.directed_by>	<m.07ydnz7>	.
<g.11b5m2ph7y>	<film.film.initial_release_date>	"1982"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b5m2ph7y>	<type.object.name>	"Crossing Over"@en	.
<g.11b5m3jh3y>	<film.film.directed_by>	<m.04mmwgg>	.
<g.11b5m3jh3y>	<film.film.initial_release_date>	"2008"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b5m3jh3y>	<type.object.name>	"Rerberg and Tarkovsky - Reverse Side of \"Stalker\""@en	.
<g.11b5m3vsgc>	<film.film.directed_by>	<m.0gw856b>	.
<g.11b5m3vsgc>	<film.film.initial_release_date>	"1961-09-15"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b5m3vsgc>	<type.object.name>	"A Daughter of a Condemned Criminal"@en	.
<g.11b5m46nh3>	<film.film.directed_by>	<m.0h4r147>	.
<g.11b5m46nh3>	<type.object.name>	"Alone Crying Star"@en	.
<g.11b5m46prp>	<film.film.directed_by>	<m.0h13lnw>	.
<g.11b5m46prp>	<type.object.name>	"Though the Heavens May Fall"@en	.
<g.11b5m7vz6g>	<film.film.directed_by>	<m.0gvdcrn>	.
<g.11b5m7vz6g>	<film.film.initial_release_date>	"1960-05-26"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b5m7vz6g>	<type.object.name>	"A Son's Judgement"@en	.
<g.11b5m9pxpy>	<film.film.directed_by>	<m.0r5gnzb>	.
<g.11b5m9pxpy>	<film.film.initial_release_date>	"1970-06-18"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b5m9pxpy>	<type.object.name>	"Born in Nampodong"@en	.
<g.11b5mb6z06>	<film.film.directed_by>	<m.05_ngk>	.
<g.11b5mb6z06>	<type.object.name>	"It's Not Me, It's Him"@en	.
<g.11b5q8crmg>	<film.film.initial_release_date>	"2011"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b5q8crmg>	<type.object.name>	"Coming Back"@en	.
<g.11b5q8crmg>	<type.object.name>	"回馬槍"@zh-Hant	.
<g.11b5q97dlq>	<film.film.directed_by>	<m.018sm6>	.
<g.11b5q97dlq>	<film.film.initial_release_date>	"1995"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b5q97dlq>	<type.object.name>	"Mr. Stitch"@en	.
<g.11b5qbn545>	<film.film.directed_by>	<m.0r3sf1k>	.
<g.11b5qbn545>	<film.film.initial_release_date>	"1973-12-27"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b5qbn545>	<type.object.name>	"The Prophet Mimi"@en	.
<g.11b5qdh4r2>	<film.film.directed_by>	<m.0d7r4qt>	.
<g.11b5qdh4r2>	<film.film.initial_release_date>	"1972-10-12"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b5qdh4r2>	<type.object.name>	"Los hijos de Satanás"@en	.
<g.11b5qdh509>	<film.director.film>	<m.0wp__3b>	.
<g.11b5qdh509>	<type.object.name>	"Jens Huckeriede"@en	.
<g.11b5qdh50p>	<film.film.directed_by>	<m.03qhxv4>	.
<g.11b5qdh50p>	<film.film.initial_release_date>	"1994"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b5qdh50p>	<type.object.name>	"Brothers: Red Roulette"@en	.
<g.11b5qdthww>	<film.film.initial_release_date>	"1968"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b5qdthww>	<type.object.name>	"Las Ruteras"@en	.
<g.11b5v1g_wp>	<film.director.film>	<m.011tzh1c>	.
<g.11b5v1g_wp>	<film.director.film>	<m.011vg5rs>	.
<g.11b5v1g_wp>	<film.director.film>	<m.0x2xx9f>	.
<g.11b5v1g_wp>	<type.object.name>	"Hiroshi Nishio"@en	.
<g.11b5v1g_wp>	<type.object.name>	"西尾 孔志"@ja	.
<g.11b5v1x6m3>	<film.film.directed_by>	<m.0fy6hb>	.
<g.11b5v1x6m3>	<film.film.initial_release_date>	"1987-04"^^<http://www.w3.org/2001/XMLSchema#gYearMonth>	.
<g.11b5v1x6m3>	<type.object.name>	"Beyoğlu'nun Arka Yakası"@en	.
<g.11b5v2q_vl>	<film.director.film>	<m.0gx2y_7>	.
<g.11b5v2q_vl>	<film.director.film>	<m.0j5jyf1>	.
<g.11b5v2q_vl>	<film.director.film>	<m.0_yvn6c>	.
<g.11b5v2q_vl>	<type.object.name>	"Marc Bauder"@en	.
<g.11b5v2w62l>	<film.film.directed_by>	<m.0b6f3cz>	.
<g.11b5v2w62l>	<film.film.initial_release_date>	"1964"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b5v2w62l>	<type.object.name>	"Agaçlar ayakta ölür"@en	.
<g.11b5v35133>	<film.director.film>	<m.05zrv7k>	.
<g.11b5v35133>	<film.director.film>	<m.0crs5yw>	.
<g.11b5v35133>	<film.director.film>	<m.0q9s5gj>	.
<g.11b5v35133>	<type.object.name>	"Halder Gomes"@en	.
<g.11b5v4810f>	<film.film.directed_by>	<m.05nczv>	.
<g.11b5v4810f>	<film.film.initial_release_date>	"2012-11-18"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b5v4810f>	<type.object.name>	"果てぬ村のミナ"@ja	.
<g.11b5vd1_8w>	<film.film.directed_by>	<m.0cv9dyx>	.
<g.11b5vd1_8w>	<film.film.initial_release_date>	"1950"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b5vd1_8w>	<type.object.name>	"Secret Document: Vienna"@en	.
<g.11b6_0dq_y>	<film.film.directed_by>	<m.0k7xl2>	.
<g.11b6_0dq_y>	<type.object.name>	"The Nightmare"@en	.
<g.11b60gm21y>	<film.film.directed_by>	<g.11bc143_kk>	.
<g.11b60gm21y>	<film.film.initial_release_date>	"1969"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b60gm21y>	<type.object.name>	"Players vs. ángeles caídos"@en	.
<g.11b60gm2xb>	<film.film.directed_by>	<m.0w0q0dy>	.
<g.11b60gm2xb>	<film.film.initial_release_date>	"1974-10-14"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b60gm2xb>	<type.object.name>	"Shayatin Elal Abad"@en	.
<g.11b60prwh6>	<film.film.directed_by>	<m.02q0dy4>	.
<g.11b60prwh6>	<film.film.initial_release_date>	"1969"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b60prwh6>	<type.object.name>	"Quiero llenarme de ti"@en	.
<g.11b60qhy11>	<film.film.directed_by>	<m.0g6ydxn>	.
<g.11b60qhy11>	<film.film.initial_release_date>	"2011-02-05"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b60qhy11>	<type.object.name>	"I Could've Been a Hooker"@en	.
<g.11b60qwcqp>	<film.film.directed_by>	<m.06zmp88>	.
<g.11b60qwcqp>	<film.film.initial_release_date>	"1978"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b60qwcqp>	<type.object.name>	"CIA contro KGB"@en	.
<g.11b60qyhzw>	<film.film.directed_by>	<m.0bbtksl>	.
<g.11b60qyhzw>	<film.film.initial_release_date>	"1969-06"^^<http://www.w3.org/2001/XMLSchema#gYearMonth>	.
<g.11b60qyhzw>	<type.object.name>	"Coup de Grâce"@en	.
<g.11b60r3k3n>	<film.director.film>	<m.0ybl2jb>	.
<g.11b60r3k3n>	<type.object.name>	"Nicolaj Pennestri"@en	.
<g.11b60rrnqx>	<film.film.directed_by>	<m.011t5694>	.
<g.11b60rrnqx>	<film.film.initial_release_date>	"2014-02-09"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b60rrnqx>	<type.object.name>	"Monooki no Piano"@en	.
<g.11b60rrnqx>	<type.object.name>	"物置のピアノ"@ja	.
<g.11b60s1ylg>	<film.director.film>	<m.0gxvk9_>	.
<g.11b60s1ylg>	<film.director.film>	<m.0j5yk2w>	.
<g.11b60s1ylg>	<type.object.name>	"Cornelia Grünberg"@en	.
<g.11b60sjzvy>	<film.director.film>	<m.010gfz5y>	.
<g.11b60sjzvy>	<film.director.film>	<m.011ty2zh>	.
<g.11b60sjzvy>	<film.director.film>	<m.011ty5ps>	.
<g.11b60sjzvy>	<film.director.film>	<m.011ty7d_>	.
<g.11b60sjzvy>	<film.director.film>	<m.011vjgdb>	.
<g.11b60sjzvy>	<film.director.film>	<m.011vkl51>	.
<g.11b60sjzvy>	<film.director.film>	<m.011vkl5k>	.
<g.11b60sjzvy>	<film.director.film>	<m.011vkl5r>	.
<g.11b60sjzvy>	<film.director.film>	<m.0crx28q>	.
<g.11b60sjzvy>	<type.object.name>	"Seiichi Fukuda"@en	.
<g.11b60sjzvy>	<type.object.name>	"福田 晴一"@ja	.
<g.11b60srh29>	<film.director.film>	<m.0gybp4n>	.
<g.11b60srh29>	<type.object.name>	"Roland Steiner"@en	.
<g.11b60tf0z8>	<film.film.directed_by>	<m.0b7bmb5>	.
<g.11b60tf0z8>	<film.film.initial_release_date>	"1998-09-09"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b60tf0z8>	<type.object.name>	"Le comptoir"@en	.
<g.11b60thryz>	<film.director.film>	<m.0w0n88s>	.
<g.11b60thryz>	<film.director.film>	<m.0w0nb1d>	.
<g.11b60thryz>	<film.director.film>	<m.0w0nd3c>	.
<g.11b60thryz>	<type.object.name>	"Iura Luncasu"@en	.
<g.11b60tlhrp>	<film.film.directed_by>	<m.027ts6t>	.
<g.11b60tlhrp>	<film.film.directed_by>	<m.03c57n3>	.
<g.11b60tlhrp>	<film.film.initial_release_date>	"2011-06-01"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b60tlhrp>	<type.object.name>	"Mangrove"@en	.
<g.11b60wv2lj>	<film.director.film>	<m.0gzlftr>	.
<g.11b60wv2lj>	<type.object.name>	"Dietmar Buchmann"@en	.
<g.11b60wv2xv>	<film.director.film>	<m.0gybk0s>	.
<g.11b60wv2xv>	<film.director.film>	<m.0w0jr0z>	.
<g.11b60wv2xv>	<type.object.name>	"Julius Matula"@en	.
<g.11b60wzbmr>	<film.film.directed_by>	<m.02pnt0k>	.
<g.11b60wzbmr>	<film.film.initial_release_date>	"1969-05-22"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b60wzbmr>	<type.object.name>	"Amor libre"@en	.
<g.11b60wzc7t>	<film.director.film>	<m.0_qzvv4>	.
<g.11b60wzc7t>	<type.object.name>	"Michihito Fujii"@en	.
<g.11b60wzc7t>	<type.object.name>	"藤井 道人"@ja	.
<g.11b60xjbjk>	<film.film.directed_by>	<m.0284srx>	.
<g.11b60xjbjk>	<film.film.initial_release_date>	"1968"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b60xjbjk>	<type.object.name>	"Matrimonio a la argentina"@en	.
<g.11b60xsczb>	<film.director.film>	<m.0glz1xk>	.
<g.11b60xsczb>	<type.object.name>	"Martin Dostál"@en	.
<g.11b60z07j7>	<film.film.directed_by>	<g.121j6lw1>	.
<g.11b60z07j7>	<film.film.initial_release_date>	"2013-09-06"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b60z07j7>	<type.object.name>	"The Tough Guys"@en	.
<g.11b6_1jt57>	<film.film.directed_by>	<m.012njh_z>	.
<g.11b6_1jt57>	<type.object.name>	"Princess"@en	.
<g.11b6_299hx>	<film.film.directed_by>	<m.02wbt4>	.
<g.11b6_299hx>	<type.object.name>	"Hellions"@en	.
<g.11b62_b98z>	<film.film.directed_by>	<m.0gjc00z>	.
<g.11b62_b98z>	<film.film.initial_release_date>	"1964"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b62_b98z>	<type.object.name>	"Le dernier tiercé"@en	.
<g.11b62rvn_w>	<film.director.film>	<m.0dlkvhq>	.
<g.11b62rvn_w>	<film.director.film>	<m.0gxb6c3>	.
<g.11b62rvn_w>	<type.object.name>	"Véronique Reymond"@en	.
<g.11b62ss3md>	<film.film.directed_by>	<m.0y_6scm>	.
<g.11b62ss3md>	<film.film.initial_release_date>	"2012-11-15"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b62ss3md>	<type.object.name>	"La fleur de l'âge"@en	.
<g.11b62ss3ms>	<film.film.directed_by>	<m.03c8bhd>	.
<g.11b62ss3ms>	<film.film.initial_release_date>	"1976-02-21"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b62ss3ms>	<type.object.name>	"わたしのSEX白書 絶頂度"@ja	.
<g.11b62ss3ms>	<type.object.name>	"Watashi no sex-hakusho"@en	.
<g.11b62t1pq1>	<film.film.directed_by>	<m.0c8cfgy>	.
<g.11b62t1pq1>	<type.object.name>	"In the Heart"@en	.
<g.11b62vqp74>	<film.film.directed_by>	<m.0b4cqpp>	.
<g.11b62vqp74>	<film.film.initial_release_date>	"2001-04-01"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b62vqp74>	<type.object.name>	"And Never Let Her Go"@en	.
<g.11b62ymfpw>	<film.film.directed_by>	<m.0114b2_q>	.
<g.11b62ymfpw>	<film.film.directed_by>	<m.0114b30h>	.
<g.11b62ymfpw>	<film.film.directed_by>	<m.0114b31v>	.
<g.11b62ymfpw>	<film.film.initial_release_date>	"2014-01-29"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b62ymfpw>	<type.object.name>	"I'm a Fucking Panther"@en	.
<g.11b62yv7_n>	<film.director.film>	<m.0109j8hw>	.
<g.11b62yv7_n>	<film.director.film>	<m.0zbg4nj>	.
<g.11b62yv7_n>	<film.director.film>	<m.0zc7g_x>	.
<g.11b62yv7_n>	<film.director.film>	<m.0zd9yr9>	.
<g.11b62yv7_n>	<type.object.name>	"José Luis Urquieta"@en	.
<g.11b632l0_v>	<film.film.directed_by>	<m.0pb66wc>	.
<g.11b632l0_v>	<film.film.initial_release_date>	"1969-02-28"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b632l0_v>	<type.object.name>	"Tango"@en	.
<g.11b639p39l>	<film.film.directed_by>	<m.09gh5f2>	.
<g.11b639p39l>	<film.film.initial_release_date>	"2008-05-23"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b639p39l>	<type.object.name>	"Bientôt j'arrête"@en	.
<g.11b63f5ncb>	<film.film.directed_by>	<m.060p6h>	.
<g.11b63f5ncb>	<film.film.initial_release_date>	"1971"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b63f5ncb>	<type.object.name>	"Out of the Darkness"@en	.
<g.11b6_482vp>	<film.film.directed_by>	<g.11bc15x6zt>	.
<g.11b6_482vp>	<film.film.initial_release_date>	"1976"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6_482vp>	<type.object.name>	"Los Cuatro secretos"@en	.
<g.11b66c_rjf>	<film.film.directed_by>	<m.09gcrwp>	.
<g.11b66c_rjf>	<film.film.initial_release_date>	"2001"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b66c_rjf>	<type.object.name>	"At the Edge of the Earth"@en	.
<g.11b66k197n>	<film.director.film>	<m.011n9cd3>	.
<g.11b66k197n>	<film.director.film>	<m.011sgqk6>	.
<g.11b66k197n>	<type.object.name>	"Yuya Yamaguchi"@en	.
<g.11b66k197n>	<type.object.name>	"山口 雄也"@ja	.
<g.11b6_6szhw>	<film.director.film>	<m.0123mfgg>	.
<g.11b6_6szhw>	<type.object.name>	"Michael Larnell"@en	.
<g.11b6_7r_25>	<film.director.film>	<m.011__c1s>	.
<g.11b6_7r_25>	<film.director.film>	<m.011__dfl>	.
<g.11b6_7r_25>	<type.object.name>	"Ilker Çatak"@en	.
<g.11b67yc985>	<film.film.directed_by>	<m.0130rx3w>	.
<g.11b67yc985>	<type.object.name>	"Teenage Mutant Ninja Turtles: Half Shell"@en	.
<g.11b6_8jh0j>	<film.director.film>	<m.0crvmsw>	.
<g.11b6_8jh0j>	<type.object.name>	"Christian Moris Müller"@en	.
<g.11b6_8ryqg>	<film.film.directed_by>	<m.04gtsb>	.
<g.11b6_8ryqg>	<type.object.name>	"Grandma"@en	.
<g.11b69gycwz>	<film.film.directed_by>	<m.05ms17p>	.
<g.11b69gycwz>	<type.object.name>	"The Gate of Youth"@en	.
<g.11b69tkpmh>	<film.director.film>	<m.012gbrnr>	.
<g.11b69tkpmh>	<type.object.name>	"Jin Mo-young"@en	.
<g.11b6b2q8gp>	<film.director.film>	<m.09tjpj6>	.
<g.11b6b2q8gp>	<type.object.name>	"Frederico Prosperi"@en	.
<g.11b6b421cg>	<film.director.film>	<m.0ch4_9m>	.
<g.11b6b421cg>	<type.object.name>	"ヴァシリー・ジュラヴリョフ"@ja	.
<g.11b6b421cg>	<type.object.name>	"Vasili Zhuravlyov"@en	.
<g.11b6b4vt8w>	<film.film.directed_by>	<m.01264qwf>	.
<g.11b6b4vt8w>	<film.film.initial_release_date>	"2014"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6b4vt8w>	<type.object.name>	"Fuerzas Especiales"@en	.
<g.11b6b4yz64>	<film.film.directed_by>	<m.064jjy>	.
<g.11b6b4yz64>	<type.object.name>	"Deepwater Horizon"@en	.
<g.11b6b58x3_>	<film.film.directed_by>	<m.011zqwzs>	.
<g.11b6b58x3_>	<film.film.initial_release_date>	"2013-04"^^<http://www.w3.org/2001/XMLSchema#gYearMonth>	.
<g.11b6b58x3_>	<type.object.name>	"Berço Imperfeito"@en	.
<g.11b6b65nft>	<film.director.film>	<m.010bvlz5>	.
<g.11b6b65nft>	<film.director.film>	<m.0pxbjxs>	.
<g.11b6b65nft>	<type.object.name>	"Askold Kurov"@en	.
<g.11b6_b98jp>	<film.director.film>	<m.0crsq01>	.
<g.11b6_b98jp>	<type.object.name>	"Antonello Belluco"@en	.
<g.11b6bb7lcw>	<film.director.film>	<m.06_3tl8>	.
<g.11b6bb7lcw>	<type.object.name>	"Kurt Früh"@en	.
<g.11b6bh_vth>	<film.director.film>	<m.02q6vn7>	.
<g.11b6bh_vth>	<type.object.name>	"Carolina Moraes-Liu"@en	.
<g.11b6bppbyq>	<film.director.film>	<m.06zmd8j>	.
<g.11b6bppbyq>	<type.object.name>	"Silvestro Prestifilippo"@en	.
<g.11b6bq2q5l>	<film.film.directed_by>	<m.0b7m9sp>	.
<g.11b6bq2q5l>	<film.film.initial_release_date>	"2001"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6bq2q5l>	<type.object.name>	"L'étrange monsieur Joseph"@en	.
<g.11b6bwdnrb>	<film.film.directed_by>	<g.122tcq1f>	.
<g.11b6bwdnrb>	<film.film.initial_release_date>	"2008-10-23"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6bwdnrb>	<type.object.name>	"Mr. Tadano's Secret Mission: From Japan with Love"@en	.
<g.11b6bwdnrb>	<type.object.name>	"特命係長・只野仁 最後の劇場版"@ja	.
<g.11b6bwdnrb>	<type.object.name>	"特命系長只野仁:最後的劇場版"@zh-Hant	.
<g.11b6bwvp86>	<film.director.film>	<m.0gfq94f>	.
<g.11b6bwvp86>	<film.director.film>	<m.0gy8wz6>	.
<g.11b6bwvp86>	<type.object.name>	"Vladimír Kavciak"@en	.
<g.11b6bwvw8n>	<film.film.directed_by>	<m.03jwbx>	.
<g.11b6bwvw8n>	<film.film.directed_by>	<m.0tl3r8t>	.
<g.11b6bwvw8n>	<type.object.name>	"Every Last Child"@en	.
<g.11b6_bxjmy>	<film.director.film>	<m.0yjtf4b>	.
<g.11b6_bxjmy>	<type.object.name>	"Asako Kageyama"@en	.
<g.11b6_bxjmy>	<type.object.name>	"影山 あさ子"@ja	.
<g.11b6_c5ct7>	<film.film.directed_by>	<m.0w8mpdv>	.
<g.11b6_c5ct7>	<film.film.initial_release_date>	"1985"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6_c5ct7>	<type.object.name>	"Al Kaf"@en	.
<g.11b6c5nrgw>	<film.film.initial_release_date>	"1989"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6c5nrgw>	<type.object.name>	"Jan Rap en z'n maat"@en	.
<g.11b6c5_t2h>	<film.film.directed_by>	<m.0nfnc39>	.
<g.11b6c5_t2h>	<film.film.initial_release_date>	"2014"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6c5_t2h>	<type.object.name>	"Divas sievietes"@en	.
<g.11b6c90stv>	<film.director.film>	<m.0v4h_6g>	.
<g.11b6c90stv>	<type.object.name>	"Vladimir Fatyanov"@en	.
<g.11b6c9q183>	<film.director.film>	<m.012hw4t4>	.
<g.11b6c9q183>	<type.object.name>	"Frederik Steiner"@en	.
<g.11b6cbfg3p>	<film.film.directed_by>	<m.02_x0l>	.
<g.11b6cbfg3p>	<film.film.initial_release_date>	"1996"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6cbfg3p>	<type.object.name>	"The VeneXiana"@en	.
<g.11b6cc6ftz>	<film.director.film>	<m.0gtl5cr>	.
<g.11b6cc6ftz>	<type.object.name>	"Václav Křístek"@en	.
<g.11b6_cgft5>	<film.film.directed_by>	<m.0k_mdnp>	.
<g.11b6_cgft5>	<film.film.initial_release_date>	"1988"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6_cgft5>	<type.object.name>	"Ayam El Ro'ab"@en	.
<g.11b6cjk8_4>	<film.film.directed_by>	<m.0w0pyr5>	.
<g.11b6cjk8_4>	<film.film.initial_release_date>	"1993"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6cjk8_4>	<type.object.name>	"Aqwa Alregal"@en	.
<g.11b6cl0sw5>	<film.film.directed_by>	<m.05h3y66>	.
<g.11b6cl0sw5>	<film.film.initial_release_date>	"1956"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6cl0sw5>	<type.object.name>	"Paris la nuit"@en	.
<g.11b6cm5mj5>	<film.film.directed_by>	<m.0pc3tgj>	.
<g.11b6cm5mj5>	<film.film.initial_release_date>	"2013"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6cm5mj5>	<type.object.name>	"Sonny Boy & Dewdrop Girl"@en	.
<g.11b6cnjntv>	<film.film.directed_by>	<m.0jwfkjc>	.
<g.11b6cnjntv>	<film.film.initial_release_date>	"1984-07"^^<http://www.w3.org/2001/XMLSchema#gYearMonth>	.
<g.11b6cnjntv>	<type.object.name>	"The Last Stand"@en	.
<g.11b6cpq3rm>	<film.film.directed_by>	<m.0g9x8lj>	.
<g.11b6cpq3rm>	<film.film.initial_release_date>	"2014"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6cpq3rm>	<type.object.name>	"Dealer"@en	.
<g.11b6cpw8wh>	<film.film.directed_by>	<m.0wp_28z>	.
<g.11b6cpw8wh>	<film.film.initial_release_date>	"2014"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6cpw8wh>	<type.object.name>	"Männerhort"@en	.
<g.11b6_cqjjf>	<film.film.directed_by>	<m.03wc2ch>	.
<g.11b6_cqjjf>	<type.object.name>	"A Poet's Life"@en	.
<g.11b6_cql31>	<film.film.directed_by>	<m.0bfdt8z>	.
<g.11b6_cql31>	<film.film.initial_release_date>	"2014"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6_cql31>	<type.object.name>	"The Price We Pay"@en	.
<g.11b6cvnbcj>	<film.film.directed_by>	<m.0_sv8g4>	.
<g.11b6cvnbcj>	<film.film.initial_release_date>	"2013"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6cvnbcj>	<type.object.name>	"Flore, route de la Mer"@en	.
<g.11b6cy97rd>	<film.film.directed_by>	<m.0ghsnpx>	.
<g.11b6cy97rd>	<film.film.initial_release_date>	"1972-02-26"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6cy97rd>	<type.object.name>	"Hyakuman-nin no dai-gasshô"@en	.
<g.11b6cy97rd>	<type.object.name>	"百万人の大合唱"@ja	.
<g.11b6d980cx>	<film.film.directed_by>	<m.02774r6>	.
<g.11b6d980cx>	<film.film.initial_release_date>	"1995-05-30"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6d980cx>	<type.object.name>	"Stand Up"@en	.
<g.11b6dbfgl9>	<film.film.directed_by>	<m.0chw_>	.
<g.11b6dbfgl9>	<type.object.name>	"マネー・モンスター"@ja	.
<g.11b6dbfgl9>	<type.object.name>	"Money Monster"@en	.
<g.11b6dbn8wl>	<film.film.directed_by>	<m.03cp9fl>	.
<g.11b6dbn8wl>	<film.film.initial_release_date>	"2014-07-10"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6dbn8wl>	<type.object.name>	"Hungry Ghost Ritual"@en	.
<g.11b6dcl2g_>	<film.film.directed_by>	<m.0bbys0p>	.
<g.11b6dcl2g_>	<film.film.initial_release_date>	"1987"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6dcl2g_>	<type.object.name>	"The Dolphin"@en	.
<g.11b6dfdw0p>	<film.director.film>	<m.010b8hxz>	.
<g.11b6dfdw0p>	<film.director.film>	<m.010p1807>	.
<g.11b6dfdw0p>	<film.director.film>	<m.0fsyd1x>	.
<g.11b6dfdw0p>	<type.object.name>	"Naoki Katô"@en	.
<g.11b6dfdw0p>	<type.object.name>	"加藤 直輝"@ja	.
<g.11b6dn6blh>	<film.director.film>	<m.0gy7prj>	.
<g.11b6dn6blh>	<film.director.film>	<m.0xn5dqk>	.
<g.11b6dn6blh>	<film.director.film>	<m.0xn5dr6>	.
<g.11b6dn6blh>	<film.director.film>	<m.0xn5f0s>	.
<g.11b6dn6blh>	<film.director.film>	<m.0xn5lbw>	.
<g.11b6dn6blh>	<type.object.name>	"Hans-Georg Ullrich"@en	.
<g.11b6dnpcz3>	<film.film.directed_by>	<m.0fq1rlw>	.
<g.11b6dnpcz3>	<film.film.initial_release_date>	"2012"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6dnpcz3>	<type.object.name>	"Convitto Falcone"@en	.
<g.11b6dw3gk3>	<film.director.film>	<m.0gybsz0>	.
<g.11b6dw3gk3>	<type.object.name>	"Heinz Brinkmann"@en	.
<g.11b6dw3gmk>	<film.director.film>	<m.0gxs4hv>	.
<g.11b6dw3gmk>	<film.director.film>	<m.0gybtv6>	.
<g.11b6dw3gmk>	<type.object.name>	"Sibylle Schönemann"@en	.
<g.11b6dw3x_r>	<film.director.film>	<g.11b6zvrl7l>	.
<g.11b6dw3x_r>	<film.director.film>	<m.0gj4zdk>	.
<g.11b6dw3x_r>	<type.object.name>	"Martin Dušek"@en	.
<g.11b6dz9sqs>	<film.film.directed_by>	<m.0gf7c2>	.
<g.11b6dz9sqs>	<film.film.initial_release_date>	"1970"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6dz9sqs>	<type.object.name>	"Un Gaucho con plata"@en	.
<g.11b6f09ytg>	<film.director.film>	<m.0nfpxh5>	.
<g.11b6f09ytg>	<film.director.film>	<m.0yw_7qf>	.
<g.11b6f09ytg>	<type.object.name>	"Ichiro Kita"@en	.
<g.11b6f09ytg>	<type.object.name>	"喜多 一郎"@ja	.
<g.11b6f1dn_y>	<film.film.directed_by>	<m.05mtlvs>	.
<g.11b6f1dn_y>	<film.film.initial_release_date>	"1963-11-12"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6f1dn_y>	<type.object.name>	"Das große Liebesspiel"@en	.
<g.11b6fkdmky>	<film.film.directed_by>	<m.0j26pcc>	.
<g.11b6fkdmky>	<type.object.name>	"Vajrakaya"@en	.
<g.11b6fkg87p>	<film.director.film>	<m.0qps8y7>	.
<g.11b6fkg87p>	<type.object.name>	"Vsevolod Benigsen"@en	.
<g.11b6_fqj2b>	<film.film.directed_by>	<m.07fz6cq>	.
<g.11b6_fqj2b>	<film.film.initial_release_date>	"1988"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6_fqj2b>	<type.object.name>	"Chet's Romance"@en	.
<g.11b6g7xb4w>	<film.film.directed_by>	<m.077g53>	.
<g.11b6g7xb4w>	<film.film.initial_release_date>	"1986"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6g7xb4w>	<type.object.name>	"Les Traces Du Reve"@en	.
<g.11b6gcf9nx>	<film.director.film>	<m.0j9bk39>	.
<g.11b6gcf9nx>	<type.object.name>	"Verena Jahnke"@en	.
<g.11b6gcq852>	<film.film.directed_by>	<m.0rh89dc>	.
<g.11b6gcq852>	<film.film.initial_release_date>	"2013-11-17"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6gcq852>	<type.object.name>	"Videoclub"@en	.
<g.11b6gd1cdh>	<film.film.directed_by>	<m.0dm34k7>	.
<g.11b6gd1cdh>	<film.film.initial_release_date>	"2014-05-11"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6gd1cdh>	<type.object.name>	"My Mother's Future Husband"@en	.
<g.11b6ggsvmq>	<film.director.film>	<m.011n666s>	.
<g.11b6ggsvmq>	<film.director.film>	<m.0gxcchp>	.
<g.11b6ggsvmq>	<film.director.film>	<m.0_w6zmh>	.
<g.11b6ggsvmq>	<type.object.name>	"Eiji Uchida"@en	.
<g.11b6ggsvmq>	<type.object.name>	"内田 英治"@ja	.
<g.11b6ghq_3q>	<film.film.directed_by>	<m.01g4bk>	.
<g.11b6ghq_3q>	<type.object.name>	"Ryuzo and the Seven Henchmen"@en	.
<g.11b6ghq_3q>	<type.object.name>	"龍三と七人の子分たち"@ja	.
<g.11b6gjm6l4>	<film.director.film>	<m.011tf2mp>	.
<g.11b6gjm6l4>	<type.object.name>	"Koshiro Otsu"@en	.
<g.11b6gjm6l4>	<type.object.name>	"大津 幸四郎"@ja	.
<g.11b6_gmx8d>	<film.film.directed_by>	<m.02wwg4s>	.
<g.11b6_gmx8d>	<type.object.name>	"Supernatural Activity"@en	.
<g.11b6gnp5bf>	<film.film.directed_by>	<m.0gdn5d>	.
<g.11b6gnp5bf>	<film.film.initial_release_date>	"1975-11-15"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6gnp5bf>	<type.object.name>	"Es herrscht Ruhe im Land"@en	.
<g.11b6gnx6hr>	<film.film.directed_by>	<m.06zntxj>	.
<g.11b6gnx6hr>	<film.film.initial_release_date>	"1994-03-23"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6gnx6hr>	<type.object.name>	"Délit mineur"@en	.
<g.11b6gnxgtw>	<film.film.directed_by>	<m.0k6_yyl>	.
<g.11b6gnxgtw>	<film.film.initial_release_date>	"2006"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6gnxgtw>	<type.object.name>	"El síndrome de Svensson"@en	.
<g.11b6gp1cys>	<film.director.film>	<m.0gkss7d>	.
<g.11b6gp1cys>	<type.object.name>	"チャン・ヨンウ"@ja	.
<g.11b6gp1cys>	<type.object.name>	"Jang Yong-Woo"@en	.
<g.11b6gp81sw>	<film.film.directed_by>	<m.0bgk55_>	.
<g.11b6gp81sw>	<film.film.initial_release_date>	"2001"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6gp81sw>	<type.object.name>	"Cowhide"@en	.
<g.11b6gpb4l2>	<film.film.directed_by>	<m.011sg0l2>	.
<g.11b6gpb4l2>	<film.film.initial_release_date>	"2014"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6gpb4l2>	<type.object.name>	"Amanet"@en	.
<g.11b6gpbczz>	<film.film.directed_by>	<m.0y4ldtn>	.
<g.11b6gpbczz>	<film.film.initial_release_date>	"2013-03-27"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6gpbczz>	<type.object.name>	"Bingo"@en	.
<g.11b6gq69nc>	<film.film.directed_by>	<m.0crbpr>	.
<g.11b6gq69nc>	<film.film.initial_release_date>	"1951-10-04"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6gq69nc>	<type.object.name>	"Die Schuld des Dr. Homma"@en	.
<g.11b6gq6dl2>	<film.film.directed_by>	<m.0mm1q>	.
<g.11b6gq6dl2>	<type.object.name>	"Knights of the Roundtable: King Arthur"@en	.
<g.11b6gqjr51>	<film.director.film>	<m.01268prt>	.
<g.11b6gqjr51>	<type.object.name>	"Steve Vallo"@en	.
<g.11b6gqsfpx>	<film.director.film>	<m.0124t4hj>	.
<g.11b6gqsfpx>	<type.object.name>	"Jonas Govaerts"@en	.
<g.11b6gqvh8b>	<film.director.film>	<m.0wq0_ss>	.
<g.11b6gqvh8b>	<type.object.name>	"Andreas Schäfer"@en	.
<g.11b6gqwqw3>	<film.film.directed_by>	<m.0jwbp7v>	.
<g.11b6gqwqw3>	<film.film.initial_release_date>	"2004-03-19"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6gqwqw3>	<type.object.name>	"Cosi x Caso"@en	.
<g.11b6gqx67c>	<film.film.initial_release_date>	"2013"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6gryw09>	<film.film.directed_by>	<g.121vqwn2>	.
<g.11b6gryw09>	<film.film.initial_release_date>	"2001-03-23"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6gryw09>	<type.object.name>	"Diapason"@en	.
<g.11b6gsq410>	<film.director.film>	<m.01268f9c>	.
<g.11b6gsq410>	<type.object.name>	"Conor Holt"@en	.
<g.11b6gsvtyg>	<film.film.directed_by>	<m.063rzy>	.
<g.11b6gsvtyg>	<film.film.initial_release_date>	"2011-09-07"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6gsvtyg>	<type.object.name>	"The Bird"@en	.
<g.11b6hh6g37>	<film.film.directed_by>	<m.0gcxcvk>	.
<g.11b6hh6g37>	<film.film.initial_release_date>	"2007-10-31"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6hh6g37>	<type.object.name>	"Le premier cri"@en	.
<g.11b6hsy7s7>	<film.film.directed_by>	<m.06k8f2>	.
<g.11b6hsy7s7>	<film.film.initial_release_date>	"1982-03-24"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6hsy7s7>	<type.object.name>	"Chassé-croisé"@en	.
<g.11b6ht4_cy>	<film.film.directed_by>	<m.09xh4w>	.
<g.11b6ht4_cy>	<type.object.name>	"Rêve et réalité"@en	.
<g.11b6hvcjr2>	<film.film.directed_by>	<m.05p1z00>	.
<g.11b6hvcjr2>	<film.film.initial_release_date>	"1953-05-14"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6hvcjr2>	<type.object.name>	"Cenerentola"@en	.
<g.11b6hwf86w>	<film.film.directed_by>	<m.0cqkn>	.
<g.11b6hwf86w>	<film.film.initial_release_date>	"1908"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6hwf86w>	<type.object.name>	"The Good Sheperdess and the Evil Princess"@en	.
<g.11b6hwsn19>	<film.film.directed_by>	<m.0hc79s7>	.
<g.11b6hwsn19>	<film.film.initial_release_date>	"2002-06-07"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6hwsn19>	<type.object.name>	"N'Gopp"@en	.
<g.11b6j11h5k>	<film.film.directed_by>	<m.0w5t0fp>	.
<g.11b6j11h5k>	<type.object.name>	"Expelled"@en	.
<g.11b6j7z0xw>	<film.film.directed_by>	<m.0gxvl91>	.
<g.11b6j7z0xw>	<film.film.initial_release_date>	"1998"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6j7z0xw>	<type.object.name>	"Rose e pistole"@en	.
<g.11b6j86pd1>	<film.film.directed_by>	<m.0_f3fw5>	.
<g.11b6j86pd1>	<film.film.initial_release_date>	"2013-10"^^<http://www.w3.org/2001/XMLSchema#gYearMonth>	.
<g.11b6j86pd1>	<type.object.name>	"Mot"@en	.
<g.11b6j8xlqm>	<film.director.film>	<m.0pxw1jv>	.
<g.11b6j8xlqm>	<type.object.name>	"Jin Woo"@en	.
<g.11b6jgvq2z>	<film.film.directed_by>	<m.0cqkn>	.
<g.11b6jgvq2z>	<film.film.initial_release_date>	"1908"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6jgvq2z>	<type.object.name>	"The New Lord of the Village"@en	.
<g.11b6jjt0lm>	<film.film.directed_by>	<g.11bbqwc044>	.
<g.11b6jjt0lm>	<film.film.initial_release_date>	"1968-02-09"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6jjt0lm>	<type.object.name>	"Rikugun chôhô 33"@en	.
<g.11b6jjt0lm>	<type.object.name>	"陸軍諜報33"@ja	.
<g.11b6js33vz>	<film.film.directed_by>	<m.03rg1j>	.
<g.11b6js33vz>	<film.film.directed_by>	<m.0gk_3j3>	.
<g.11b6js33vz>	<type.object.name>	"The \"Teddy\" Bears"@en	.
<g.11b6_l0550>	<film.film.directed_by>	<m.02qyyfv>	.
<g.11b6_l0550>	<type.object.name>	"Unexpected"@en	.
<g.11b6m5bqjw>	<film.director.film>	<g.12z65vm8y>	.
<g.11b6m5bqjw>	<type.object.name>	"Renaud Victor"@en	.
<g.11b6m615sj>	<film.director.film>	<m.0c4d4m2>	.
<g.11b6m615sj>	<film.director.film>	<m.0q1_ymy>	.
<g.11b6m615sj>	<type.object.name>	"Tony D'Angelo"@en	.
<g.11b6_mh4wn>	<film.director.film>	<m.063hbqp>	.
<g.11b6_mh4wn>	<film.director.film>	<m.063qr_d>	.
<g.11b6_mh4wn>	<film.director.film>	<m.063thhn>	.
<g.11b6_mh4wn>	<film.director.film>	<m.063tjj7>	.
<g.11b6_mh4wn>	<film.director.film>	<m.065w9xg>	.
<g.11b6_mh4wn>	<film.director.film>	<m.0663p_t>	.
<g.11b6_mh4wn>	<type.object.name>	"Garth Evans"@en	.
<g.11b6mt7qxr>	<film.film.directed_by>	<m.03cr9js>	.
<g.11b6mt7qxr>	<film.film.initial_release_date>	"1979-08-22"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6mt7qxr>	<type.object.name>	"Dumb But Disciplined"@en	.
<g.11b6_n6p_h>	<film.film.directed_by>	<m.0798xq>	.
<g.11b6_n6p_h>	<film.film.initial_release_date>	"1970"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6_n6p_h>	<type.object.name>	"Abracadabra"@en	.
<g.11b6njm7yk>	<film.film.initial_release_date>	"2014"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6njm7yk>	<type.object.name>	"Tell"@en	.
<g.11b6nmvbqp>	<film.director.film>	<m.010p5lk9>	.
<g.11b6nmvbqp>	<type.object.name>	"Akihiro Toda"@en	.
<g.11b6nmvbqp>	<type.object.name>	"戸田 彬弘"@ja	.
<g.11b6_p5lk6>	<film.director.film>	<m.011ms9vd>	.
<g.11b6_p5lk6>	<type.object.name>	"Jake Helgren"@en	.
<g.11b6phnl23>	<film.director.film>	<m.010nx8xr>	.
<g.11b6phnl23>	<type.object.name>	"Shinya Nishimura"@en	.
<g.11b6phnl23>	<type.object.name>	"西村 晋也"@ja	.
<g.11b6pl6s0w>	<film.film.directed_by>	<m.0k6929>	.
<g.11b6pl6s0w>	<film.film.initial_release_date>	"1996-03-24"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6pl6s0w>	<type.object.name>	"Pêcheur d'Islande"@en	.
<g.11b6pmg5sy>	<film.film.directed_by>	<m.02pnt0k>	.
<g.11b6pmg5sy>	<film.film.initial_release_date>	"1973"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6pmg5sy>	<type.object.name>	"El Mundo que inventamos"@en	.
<g.11b6pqft8n>	<film.film.directed_by>	<g.11bwshmy6q>	.
<g.11b6pqft8n>	<film.film.initial_release_date>	"2010"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6pqft8n>	<type.object.name>	"Your Soon Erdal"@en	.
<g.11b6r532v5>	<film.film.directed_by>	<m.012hw3xz>	.
<g.11b6r532v5>	<film.film.initial_release_date>	"2013"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6r532v5>	<type.object.name>	"High Five"@en	.
<g.11b6_r7mbb>	<film.film.directed_by>	<m.0s9hqz4>	.
<g.11b6_r7mbb>	<film.film.initial_release_date>	"1988"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6_r7mbb>	<type.object.name>	"L'escalier chimérique"@en	.
<g.11b6_r_7r0>	<film.director.film>	<m.012c5dmp>	.
<g.11b6_r_7r0>	<type.object.name>	"André van der Hout"@en	.
<g.11b6rc7tmj>	<film.film.directed_by>	<m.04j3yt>	.
<g.11b6rc7tmj>	<film.film.initial_release_date>	"1969"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6rc7tmj>	<type.object.name>	"The Devil by the Tail"@en	.
<g.11b6rzctk7>	<film.director.film>	<m.0gxb081>	.
<g.11b6rzctk7>	<film.director.film>	<m.0gy7v1v>	.
<g.11b6rzctk7>	<type.object.name>	"Reni Mertens"@en	.
<g.11b6snv7sk>	<film.film.directed_by>	<m.0bg4q8>	.
<g.11b6snv7sk>	<film.film.initial_release_date>	"1929-01-19"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6snv7sk>	<type.object.name>	"Deaf Sam-Ryong"@en	.
<g.11b6sw4gy9>	<film.film.directed_by>	<m.02q_rs4>	.
<g.11b6sw4gy9>	<film.film.initial_release_date>	"1968"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6sw4gy9>	<type.object.name>	"Kidnapping of the Sun and Moon"@en	.
<g.11b6sw6gw1>	<film.film.directed_by>	<m.0p2jr>	.
<g.11b6sw6gw1>	<film.film.initial_release_date>	"1939-12"^^<http://www.w3.org/2001/XMLSchema#gYearMonth>	.
<g.11b6sw6gw1>	<type.object.name>	"Without Tomorrow"@en	.
<g.11b6_szgk1>	<film.film.directed_by>	<m.051p44>	.
<g.11b6_szgk1>	<type.object.name>	"The Sound of Music"@en	.
<g.11b6szjzn_>	<film.director.film>	<m.0y7p01g>	.
<g.11b6szjzn_>	<type.object.name>	"Gaigals Laimons"@en	.
<g.11b6t1scwz>	<film.film.directed_by>	<g.11bc16p_ch>	.
<g.11b6t1scwz>	<film.film.initial_release_date>	"1973"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6t1scwz>	<type.object.name>	"José María y María José: Una pareja de hoy"@en	.
<g.11b6t340fz>	<film.film.directed_by>	<m.0cggxcn>	.
<g.11b6t340fz>	<film.film.initial_release_date>	"2004"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6t340fz>	<type.object.name>	"Alma mater"@en	.
<g.11b6t_48yh>	<film.director.film>	<m.0gy7tw5>	.
<g.11b6t_48yh>	<film.director.film>	<m.0gzlpfm>	.
<g.11b6t_48yh>	<type.object.name>	"Marianne Rosenbaum"@en	.
<g.11b6t54c06>	<film.film.directed_by>	<m.012bnp8b>	.
<g.11b6t54c06>	<film.film.directed_by>	<m.012bnpb0>	.
<g.11b6t54c06>	<film.film.initial_release_date>	"2014"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6t54c06>	<type.object.name>	"Ri̇molar Ve Zi̇molar - Kasabada Bariş"@en	.
<g.11b6t_6drx>	<film.film.directed_by>	<m.0x1shfs>	.
<g.11b6t7kd72>	<film.film.directed_by>	<m.0flxq9>	.
<g.11b6t7kd72>	<film.film.initial_release_date>	"1996"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6t7kd72>	<type.object.name>	"Entrance of the P-Side"@en	.
<g.11b6t7lwyv>	<film.film.directed_by>	<g.11bc16t95s>	.
<g.11b6_t8gr_>	<film.director.film>	<m.0w2tdnv>	.
<g.11b6_t8gr_>	<type.object.name>	"Per-Olav Sørensen"@en	.
<g.11b6tcdhzs>	<film.film.directed_by>	<m.06_xmzl>	.
<g.11b6tcdhzs>	<film.film.initial_release_date>	"1981"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6tcdhzs>	<type.object.name>	"681 AD: The Glory of Khan"@en	.
<g.11b6tcdhzs>	<type.object.name>	"汗（カーン）の栄光"@ja	.
<g.11b6tgp1m9>	<film.film.directed_by>	<m.0dm390m>	.
<g.11b6tgp1m9>	<film.film.initial_release_date>	"2002"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6tgp1m9>	<type.object.name>	"Twilight"@en	.
<g.11b6tt_r70>	<film.director.film>	<m.012hdpqg>	.
<g.11b6tt_r70>	<type.object.name>	"Paul J. Lane"@en	.
<g.11b6tvmjsc>	<film.film.directed_by>	<m.0hzytpx>	.
<g.11b6tvmjsc>	<film.film.initial_release_date>	"2012-01-28"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6tvmjsc>	<type.object.name>	"What We'll Leave Behind"@en	.
<g.11b6tvmjsc>	<type.object.name>	"僕たちに残されるもの"@ja	.
<g.11b6v4136_>	<film.film.directed_by>	<m.03cv5hr>	.
<g.11b6v4136_>	<type.object.name>	"Let Hoi Decide"@en	.
<g.11b6v6xhqq>	<film.director.film>	<m.0wq0rf5>	.
<g.11b6v6xhqq>	<type.object.name>	"Stephan Mayer"@en	.
<g.11b6_v8t6_>	<film.film.directed_by>	<m.06_4_2b>	.
<g.11b6_v8t6_>	<film.film.initial_release_date>	"2002"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6_v8t6_>	<type.object.name>	"A Summer Night Rendez-vous"@en	.
<g.11b6vfh1rz>	<film.director.film>	<m.0ch5ftj>	.
<g.11b6vfh1rz>	<type.object.name>	"Georges Grammat"@en	.
<g.11b6vjj1rv>	<film.director.film>	<g.1hc0hl6sl>	.
<g.11b6vjj1rv>	<film.director.film>	<m.0125gt1r>	.
<g.11b6vjj1rv>	<film.director.film>	<m.0qp_n71>	.
<g.11b6vjj1rv>	<type.object.name>	"Rouhollah Hejazi"@en	.
<g.11b6vl04r0>	<film.film.directed_by>	<m.0798xq>	.
<g.11b6vl04r0>	<film.film.initial_release_date>	"1972"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6vl04r0>	<type.object.name>	"The creation of birds"@en	.
<g.11b6vms6cz>	<film.director.film>	<m.0w1lt9m>	.
<g.11b6vms6cz>	<type.object.name>	"Hermann Lanske"@en	.
<g.11b6vvjp4g>	<film.film.directed_by>	<m.053_vl>	.
<g.11b6vvjp4g>	<film.film.initial_release_date>	"2013"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6vvjp4g>	<type.object.name>	"Cine Gibi 6"@en	.
<g.11b6_vxps7>	<film.director.film>	<m.099l1r>	.
<g.11b6_vxps7>	<film.director.film>	<m.0bvc176>	.
<g.11b6_vxps7>	<film.director.film>	<m.0j2mzlw>	.
<g.11b6_vxps7>	<type.object.name>	"Estela Bravo"@en	.
<g.11b6vyscgj>	<film.film.directed_by>	<m.0bqsc3m>	.
<g.11b6vyscgj>	<film.film.initial_release_date>	"2002"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6vyscgj>	<type.object.name>	"Hunting Down An Angel or Four Passions of the Soothsayer Poet"@en	.
<g.11b6vz8dpr>	<film.director.film>	<m.011sg200>	.
<g.11b6vz8dpr>	<film.director.film>	<m.0bnnr8y>	.
<g.11b6vz8dpr>	<film.director.film>	<m.0gf8qkc>	.
<g.11b6vz8dpr>	<type.object.name>	"Anka Schmid"@en	.
<g.11b6wf8mq2>	<film.film.directed_by>	<m.05b_r8f>	.
<g.11b6wf8mq2>	<film.film.initial_release_date>	"1958-08-12"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6wf8mq2>	<type.object.name>	"Hatamoto taikutsu otoko"@en	.
<g.11b6wf8mq2>	<type.object.name>	"旗本退屈男"@ja	.
<g.11b6x2g13n>	<film.director.film>	<m.0121_qlt>	.
<g.11b6x2g13n>	<film.director.film>	<m.047q09j>	.
<g.11b6x2g13n>	<film.director.film>	<m.0kxg0_n>	.
<g.11b6x2g13n>	<type.object.name>	"Naoyoshi Shiotani"@en	.
<g.11b6x2xh91>	<film.director.film>	<m.0bf2mhv>	.
<g.11b6x2xh91>	<film.director.film>	<m.0bf3b4w>	.
<g.11b6x2xh91>	<film.director.film>	<m.0vpkpd5>	.
<g.11b6x2xh91>	<type.object.name>	"Antonio Racioppi"@en	.
<g.11b6x5183g>	<film.film.directed_by>	<m.06w46n8>	.
<g.11b6x5183g>	<film.film.initial_release_date>	"2003"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6x5183g>	<type.object.name>	"Zohre & Manouchehr"@en	.
<g.11b6x9drz8>	<film.film.directed_by>	<m.0s94s7w>	.
<g.11b6x9drz8>	<film.film.initial_release_date>	"1985"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6x9drz8>	<type.object.name>	"Dialogue De Sourds"@en	.
<g.11b6x9yb42>	<film.film.directed_by>	<m.0cglxw8>	.
<g.11b6x9yb42>	<type.object.name>	"Prim, El Asesinato De La Calle Del Turco"@en	.
<g.11b6x9zcw8>	<film.director.film>	<m.010r18zp>	.
<g.11b6x9zcw8>	<film.director.film>	<m.04tk_gv>	.
<g.11b6x9zcw8>	<film.director.film>	<m.0zyb4f5>	.
<g.11b6x9zcw8>	<type.object.name>	"Tomek Ducki"@en	.
<g.11b6xb794b>	<film.film.directed_by>	<m.0cqkn>	.
<g.11b6xb794b>	<type.object.name>	"Punch and Judy"@en	.
<g.11b6xbvfk5>	<film.film.directed_by>	<m.05f5ntj>	.
<g.11b6xbvfk5>	<film.film.initial_release_date>	"1977"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6xbvfk5>	<type.object.name>	"La maréchal-ferrant"@en	.
<g.11b6xdcddt>	<film.film.directed_by>	<m.025sk9>	.
<g.11b6xdcddt>	<film.film.initial_release_date>	"1953"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6xdcddt>	<type.object.name>	"Voice of Silence"@en	.
<g.11b6xdm_9l>	<film.film.directed_by>	<m.0hp7ls8>	.
<g.11b6xdm_9l>	<film.film.initial_release_date>	"2009-11-28"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6xdm_9l>	<type.object.name>	"Just for Sex"@en	.
<g.11b6xdssyb>	<film.film.directed_by>	<m.0hcg29c>	.
<g.11b6xdssyb>	<film.film.initial_release_date>	"1982"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6xdssyb>	<type.object.name>	"Bluff"@en	.
<g.11b6xdyz9z>	<film.director.film>	<m.0gmlpl7>	.
<g.11b6xdyz9z>	<film.director.film>	<m.0qs89z3>	.
<g.11b6xdyz9z>	<type.object.name>	"Michal Marczak"@en	.
<g.11b6xdyzbq>	<film.director.film>	<g.11jgdvg5y>	.
<g.11b6xdyzbq>	<film.director.film>	<g.11x7vq19f>	.
<g.11b6xdyzbq>	<film.director.film>	<g.12196108>	.
<g.11b6xdyzbq>	<film.director.film>	<g.1pxxnzhyz>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0v10jj8>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0v23wtd>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0v2466h>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0v2fjpm>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0v2lcyz>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0v2ldfw>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0v4z3sb>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0v4zf9h>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0v96mr_>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vbdpwk>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vmwvpv>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vn_v49>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vpcfln>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vpcfqq>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vpcgxx>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vt297m>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vt2c3g>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vt2g0h>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vt2m4_>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vt31dz>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vt32nw>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vtf82f>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0vttk6h>	.
<g.11b6xdyzbq>	<film.director.film>	<m.0w2v3b7>	.
<g.11b6xdyzbq>	<type.object.name>	"Aram Gülyüz"@en	.
<g.11b6xhcxcs>	<film.film.directed_by>	<m.011lkthy>	.
<g.11b6xhcxcs>	<film.film.initial_release_date>	"2012-05-05"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6xhcxcs>	<type.object.name>	"Sins Expiation"@en	.
<g.11b6xmcbhv>	<film.film.directed_by>	<m.0gtk06v>	.
<g.11b6xmcbhv>	<film.film.initial_release_date>	"2004-09-10"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6xmcbhv>	<type.object.name>	"Doma Ahn Jung-geun"@en	.
<g.11b6y0jf9s>	<film.director.film>	<m.010qqt9f>	.
<g.11b6y0jf9s>	<film.director.film>	<m.09z6j3>	.
<g.11b6y0jf9s>	<film.director.film>	<m.0gksxbw>	.
<g.11b6y0jf9s>	<type.object.name>	"Byeong-Kuk Hwang"@en	.
<g.11b6y14v_y>	<film.director.film>	<m.09xy4c>	.
<g.11b6y14v_y>	<film.director.film>	<m.09z69v>	.
<g.11b6y14v_y>	<film.director.film>	<m.0kv8s6>	.
<g.11b6y14v_y>	<film.director.film>	<m.0w7ptnq>	.
<g.11b6y14v_y>	<type.object.name>	"Park Heung-shik"@en	.
<g.11b6y6j4rq>	<film.director.film>	<m.0vtp13c>	.
<g.11b6y6j4rq>	<film.director.film>	<m.0yj9glz>	.
<g.11b6y6j4rq>	<type.object.name>	"Ario Rubbik"@en	.
<g.11b6y9tkn4>	<film.film.directed_by>	<m.027kbnt>	.
<g.11b6y9tkn4>	<type.object.name>	"The Revised Fundamentals of Caregiving"@en	.
<g.11b6yd6fv_>	<film.director.film>	<m.0wp_fk1>	.
<g.11b6yd6fv_>	<type.object.name>	"Andreas Senn"@en	.
<g.11b6ylyym4>	<film.director.film>	<m.04j09zt>	.
<g.11b6ylyym4>	<film.director.film>	<m.04j2yqy>	.
<g.11b6ylyym4>	<film.director.film>	<m.0gxnt1r>	.
<g.11b6ylyym4>	<type.object.name>	"ジョン・ハドルズ"@ja	.
<g.11b6ylyym4>	<type.object.name>	"John Huddles"@en	.
<g.11b6ywqgrl>	<film.director.film>	<m.0y848sh>	.
<g.11b6ywqgrl>	<type.object.name>	"Bunji Sotoyama"@en	.
<g.11b6ywqgrl>	<type.object.name>	"外山 文治"@ja	.
<g.11b6z295dz>	<film.film.directed_by>	<g.11bc149p88>	.
<g.11b6z_j4c8>	<film.film.directed_by>	<g.122g3vmx>	.
<g.11b6z_j4c8>	<film.film.initial_release_date>	"1987"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6z_j4c8>	<type.object.name>	"L'été perdu"@en	.
<g.11b6zntyf8>	<film.director.film>	<g.120mbrk3>	.
<g.11b6zntyf8>	<film.director.film>	<m.076zzpc>	.
<g.11b6zntyf8>	<type.object.name>	"Bernard Favre"@en	.
<g.11b6_zpv3f>	<film.film.directed_by>	<m.02796s6>	.
<g.11b6_zpv3f>	<film.film.initial_release_date>	"1971-03-24"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6_zpv3f>	<type.object.name>	"A Little, a Lot, Passionately"@en	.
<g.11b6zsbwty>	<film.director.film>	<m.0z46ngw>	.
<g.11b6zsbwty>	<type.object.name>	"Magne Pettersen"@en	.
<g.11b6zt7yq2>	<film.film.directed_by>	<m.059_zgx>	.
<g.11b6zt7yq2>	<film.film.initial_release_date>	"1964"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6zt7yq2>	<type.object.name>	"Mort, où est ta victoire?"@en	.
<g.11b6zvrj5g>	<film.film.directed_by>	<m.0bj5zdy>	.
<g.11b6zvrj5g>	<type.object.name>	"I'll See You in My Dreams"@en	.
<g.11b6zvrl7l>	<film.film.directed_by>	<g.11b6dw3x_r>	.
<g.11b6zvrl7l>	<film.film.initial_release_date>	"2014-07-08"^^<http://www.w3.org/2001/XMLSchema#date>	.
<g.11b6zvrl7l>	<type.object.name>	"Into the Clouds We Gaze"@en	.
<g.11b6zxhw2v>	<film.film.directed_by>	<m.0k3f5r>	.
<g.11b6zxhw2v>	<film.film.initial_release_date>	"1990"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b6zxhw2v>	<type.object.name>	"Zkouskové období"@en	.
<g.11b6zzvcjm>	<film.director.film>	<m.0t53mb5>	.
<g.11b6zzvcjm>	<type.object.name>	"シャオ・リーショウ"@ja	.
<g.11b6zzvcjm>	<type.object.name>	"Li-Xiu Xiao"@en	.
<g.11b6zzvcjm>	<type.object.name>	"蕭力修"@zh-Hant	.
<g.11b707hsp3>	<film.film.directed_by>	<m.04mkgkq>	.
<g.11b707hsp3>	<film.film.directed_by>	<m.04mkgp1>	.
<g.11b707hsp3>	<film.film.initial_release_date>	"2009"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b707hsp3>	<type.object.name>	"It’s Free For Girls"@en	.
<g.11b70_c44m>	<film.film.directed_by>	<m.0bbbf7r>	.
<g.11b70_c44m>	<film.film.initial_release_date>	"1983"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b70_c44m>	<type.object.name>	"Star suburb: La banlieue des étoiles"@en	.
<g.11b70cbl3z>	<film.film.directed_by>	<g.11bc16kwcm>	.
<g.11b70cbl3z>	<film.film.initial_release_date>	"1975"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b70cbl3z>	<type.object.name>	"Los Chantas"@en	.
<g.11b70ftv76>	<film.film.directed_by>	<g.11bc14f41d>	.
<g.11b70ftv76>	<film.film.directed_by>	<m.02x413c>	.
<g.11b70ftv76>	<film.film.initial_release_date>	"1975"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b70ftv76>	<type.object.name>	"Un Mundo de amor"@en	.
<g.11b70kxbr_>	<film.film.directed_by>	<m.02rjpgn>	.
<g.11b70kxbr_>	<film.film.initial_release_date>	"2014"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b70kxbr_>	<type.object.name>	"A Woman as a Friend"@en	.
<g.11b71x5zy1>	<film.film.directed_by>	<m.0gvn8v6>	.
<g.11b71x5zy1>	<film.film.initial_release_date>	"2013"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b71x5zy1>	<type.object.name>	"Au Sol"@en	.
<g.11b720r0m7>	<film.film.directed_by>	<m.03qcz3j>	.
<g.11b720r0m7>	<film.film.initial_release_date>	"1929"^^<http://www.w3.org/2001/XMLSchema#gYear>	.
<g.11b720r0m7>	<type.object.name>	"Autumn Mists"@en	.
<g.11b7232489>	<film.director.film>	<m.0b76pxf>	.
<g.11b7232489>	<film.director.film>	<m.0b77vkz>	.
<g.11b7232489>	<type.object.name>	"Alper Mestçi"@en	.
<g.11b725m1sy>	<film.director.film>	<m.0cr_m43>	.
<g.11b725m1sy>	<film.director.film>	<m.0crsr6l>	.
	}
}`
