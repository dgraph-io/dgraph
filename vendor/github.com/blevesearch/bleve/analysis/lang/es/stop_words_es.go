package es

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_es"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/snowball/
// ` was changed to ' to allow for literal string

var SpanishStopWords = []byte(` | From svn.tartarus.org/snowball/trunk/website/algorithms/spanish/stop.txt
 | This file is distributed under the BSD License.
 | See http://snowball.tartarus.org/license.php
 | Also see http://www.opensource.org/licenses/bsd-license.html
 |  - Encoding was converted to UTF-8.
 |  - This notice was added.
 |
 | NOTE: To use this file with StopFilterFactory, you must specify format="snowball"

 | A Spanish stop word list. Comments begin with vertical bar. Each stop
 | word is at the start of a line.


 | The following is a ranked list (commonest to rarest) of stopwords
 | deriving from a large sample of text.

 | Extra words have been added at the end.

de             |  from, of
la             |  the, her
que            |  who, that
el             |  the
en             |  in
y              |  and
a              |  to
los            |  the, them
del            |  de + el
se             |  himself, from him etc
las            |  the, them
por            |  for, by, etc
un             |  a
para           |  for
con            |  with
no             |  no
una            |  a
su             |  his, her
al             |  a + el
  | es         from SER
lo             |  him
como           |  how
más            |  more
pero           |  pero
sus            |  su plural
le             |  to him, her
ya             |  already
o              |  or
  | fue        from SER
este           |  this
  | ha         from HABER
sí             |  himself etc
porque         |  because
esta           |  this
  | son        from SER
entre          |  between
  | está     from ESTAR
cuando         |  when
muy            |  very
sin            |  without
sobre          |  on
  | ser        from SER
  | tiene      from TENER
también        |  also
me             |  me
hasta          |  until
hay            |  there is/are
donde          |  where
  | han        from HABER
quien          |  whom, that
  | están      from ESTAR
  | estado     from ESTAR
desde          |  from
todo           |  all
nos            |  us
durante        |  during
  | estados    from ESTAR
todos          |  all
uno            |  a
les            |  to them
ni             |  nor
contra         |  against
otros          |  other
  | fueron     from SER
ese            |  that
eso            |  that
  | había      from HABER
ante           |  before
ellos          |  they
e              |  and (variant of y)
esto           |  this
mí             |  me
antes          |  before
algunos        |  some
qué            |  what?
unos           |  a
yo             |  I
otro           |  other
otras          |  other
otra           |  other
él             |  he
tanto          |  so much, many
esa            |  that
estos          |  these
mucho          |  much, many
quienes        |  who
nada           |  nothing
muchos         |  many
cual           |  who
  | sea        from SER
poco           |  few
ella           |  she
estar          |  to be
  | haber      from HABER
estas          |  these
  | estaba     from ESTAR
  | estamos    from ESTAR
algunas        |  some
algo           |  something
nosotros       |  we

      | other forms

mi             |  me
mis            |  mi plural
tú             |  thou
te             |  thee
ti             |  thee
tu             |  thy
tus            |  tu plural
ellas          |  they
nosotras       |  we
vosotros       |  you
vosotras       |  you
os             |  you
mío            |  mine
mía            |
míos           |
mías           |
tuyo           |  thine
tuya           |
tuyos          |
tuyas          |
suyo           |  his, hers, theirs
suya           |
suyos          |
suyas          |
nuestro        |  ours
nuestra        |
nuestros       |
nuestras       |
vuestro        |  yours
vuestra        |
vuestros       |
vuestras       |
esos           |  those
esas           |  those

               | forms of estar, to be (not including the infinitive):
estoy
estás
está
estamos
estáis
están
esté
estés
estemos
estéis
estén
estaré
estarás
estará
estaremos
estaréis
estarán
estaría
estarías
estaríamos
estaríais
estarían
estaba
estabas
estábamos
estabais
estaban
estuve
estuviste
estuvo
estuvimos
estuvisteis
estuvieron
estuviera
estuvieras
estuviéramos
estuvierais
estuvieran
estuviese
estuvieses
estuviésemos
estuvieseis
estuviesen
estando
estado
estada
estados
estadas
estad

               | forms of haber, to have (not including the infinitive):
he
has
ha
hemos
habéis
han
haya
hayas
hayamos
hayáis
hayan
habré
habrás
habrá
habremos
habréis
habrán
habría
habrías
habríamos
habríais
habrían
había
habías
habíamos
habíais
habían
hube
hubiste
hubo
hubimos
hubisteis
hubieron
hubiera
hubieras
hubiéramos
hubierais
hubieran
hubiese
hubieses
hubiésemos
hubieseis
hubiesen
habiendo
habido
habida
habidos
habidas

               | forms of ser, to be (not including the infinitive):
soy
eres
es
somos
sois
son
sea
seas
seamos
seáis
sean
seré
serás
será
seremos
seréis
serán
sería
serías
seríamos
seríais
serían
era
eras
éramos
erais
eran
fui
fuiste
fue
fuimos
fuisteis
fueron
fuera
fueras
fuéramos
fuerais
fueran
fuese
fueses
fuésemos
fueseis
fuesen
siendo
sido
  |  sed also means 'thirst'

               | forms of tener, to have (not including the infinitive):
tengo
tienes
tiene
tenemos
tenéis
tienen
tenga
tengas
tengamos
tengáis
tengan
tendré
tendrás
tendrá
tendremos
tendréis
tendrán
tendría
tendrías
tendríamos
tendríais
tendrían
tenía
tenías
teníamos
teníais
tenían
tuve
tuviste
tuvo
tuvimos
tuvisteis
tuvieron
tuviera
tuvieras
tuviéramos
tuvierais
tuvieran
tuviese
tuvieses
tuviésemos
tuvieseis
tuviesen
teniendo
tenido
tenida
tenidos
tenidas
tened

`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(SpanishStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
