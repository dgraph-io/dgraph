package pt

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_pt"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/snowball/
// ` was changed to ' to allow for literal string

var PortugueseStopWords = []byte(` | From svn.tartarus.org/snowball/trunk/website/algorithms/portuguese/stop.txt
 | This file is distributed under the BSD License.
 | See http://snowball.tartarus.org/license.php
 | Also see http://www.opensource.org/licenses/bsd-license.html
 |  - Encoding was converted to UTF-8.
 |  - This notice was added.
 |
 | NOTE: To use this file with StopFilterFactory, you must specify format="snowball"

 | A Portuguese stop word list. Comments begin with vertical bar. Each stop
 | word is at the start of a line.


 | The following is a ranked list (commonest to rarest) of stopwords
 | deriving from a large sample of text.

 | Extra words have been added at the end.

de             |  of, from
a              |  the; to, at; her
o              |  the; him
que            |  who, that
e              |  and
do             |  de + o
da             |  de + a
em             |  in
um             |  a
para           |  for
  | é          from SER
com            |  with
não            |  not, no
uma            |  a
os             |  the; them
no             |  em + o
se             |  himself etc
na             |  em + a
por            |  for
mais           |  more
as             |  the; them
dos            |  de + os
como           |  as, like
mas            |  but
  | foi        from SER
ao             |  a + o
ele            |  he
das            |  de + as
  | tem        from TER
à              |  a + a
seu            |  his
sua            |  her
ou             |  or
  | ser        from SER
quando         |  when
muito          |  much
  | há         from HAV
nos            |  em + os; us
já             |  already, now
  | está       from EST
eu             |  I
também         |  also
só             |  only, just
pelo           |  per + o
pela           |  per + a
até            |  up to
isso           |  that
ela            |  he
entre          |  between
  | era        from SER
depois         |  after
sem            |  without
mesmo          |  same
aos            |  a + os
  | ter        from TER
seus           |  his
quem           |  whom
nas            |  em + as
me             |  me
esse           |  that
eles           |  they
  | estão      from EST
você           |  you
  | tinha      from TER
  | foram      from SER
essa           |  that
num            |  em + um
nem            |  nor
suas           |  her
meu            |  my
às             |  a + as
minha          |  my
  | têm        from TER
numa           |  em + uma
pelos          |  per + os
elas           |  they
  | havia      from HAV
  | seja       from SER
qual           |  which
  | será       from SER
nós            |  we
  | tenho      from TER
lhe            |  to him, her
deles          |  of them
essas          |  those
esses          |  those
pelas          |  per + as
este           |  this
  | fosse      from SER
dele           |  of him

 | other words. There are many contractions such as naquele = em+aquele,
 | mo = me+o, but they are rare.
 | Indefinite article plural forms are also rare.

tu             |  thou
te             |  thee
vocês          |  you (plural)
vos            |  you
lhes           |  to them
meus           |  my
minhas
teu            |  thy
tua
teus
tuas
nosso          | our
nossa
nossos
nossas

dela           |  of her
delas          |  of them

esta           |  this
estes          |  these
estas          |  these
aquele         |  that
aquela         |  that
aqueles        |  those
aquelas        |  those
isto           |  this
aquilo         |  that

               | forms of estar, to be (not including the infinitive):
estou
está
estamos
estão
estive
esteve
estivemos
estiveram
estava
estávamos
estavam
estivera
estivéramos
esteja
estejamos
estejam
estivesse
estivéssemos
estivessem
estiver
estivermos
estiverem

               | forms of haver, to have (not including the infinitive):
hei
há
havemos
hão
houve
houvemos
houveram
houvera
houvéramos
haja
hajamos
hajam
houvesse
houvéssemos
houvessem
houver
houvermos
houverem
houverei
houverá
houveremos
houverão
houveria
houveríamos
houveriam

               | forms of ser, to be (not including the infinitive):
sou
somos
são
era
éramos
eram
fui
foi
fomos
foram
fora
fôramos
seja
sejamos
sejam
fosse
fôssemos
fossem
for
formos
forem
serei
será
seremos
serão
seria
seríamos
seriam

               | forms of ter, to have (not including the infinitive):
tenho
tem
temos
tém
tinha
tínhamos
tinham
tive
teve
tivemos
tiveram
tivera
tivéramos
tenha
tenhamos
tenham
tivesse
tivéssemos
tivessem
tiver
tivermos
tiverem
terei
terá
teremos
terão
teria
teríamos
teriam
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(PortugueseStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
