package it

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_it"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/snowball/
// ` was changed to ' to allow for literal string

var ItalianStopWords = []byte(` | From svn.tartarus.org/snowball/trunk/website/algorithms/italian/stop.txt
 | This file is distributed under the BSD License.
 | See http://snowball.tartarus.org/license.php
 | Also see http://www.opensource.org/licenses/bsd-license.html
 |  - Encoding was converted to UTF-8.
 |  - This notice was added.
 |
 | NOTE: To use this file with StopFilterFactory, you must specify format="snowball"

 | An Italian stop word list. Comments begin with vertical bar. Each stop
 | word is at the start of a line.

ad             |  a (to) before vowel
al             |  a + il
allo           |  a + lo
ai             |  a + i
agli           |  a + gli
all            |  a + l'
agl            |  a + gl'
alla           |  a + la
alle           |  a + le
con            |  with
col            |  con + il
coi            |  con + i (forms collo, cogli etc are now very rare)
da             |  from
dal            |  da + il
dallo          |  da + lo
dai            |  da + i
dagli          |  da + gli
dall           |  da + l'
dagl           |  da + gll'
dalla          |  da + la
dalle          |  da + le
di             |  of
del            |  di + il
dello          |  di + lo
dei            |  di + i
degli          |  di + gli
dell           |  di + l'
degl           |  di + gl'
della          |  di + la
delle          |  di + le
in             |  in
nel            |  in + el
nello          |  in + lo
nei            |  in + i
negli          |  in + gli
nell           |  in + l'
negl           |  in + gl'
nella          |  in + la
nelle          |  in + le
su             |  on
sul            |  su + il
sullo          |  su + lo
sui            |  su + i
sugli          |  su + gli
sull           |  su + l'
sugl           |  su + gl'
sulla          |  su + la
sulle          |  su + le
per            |  through, by
tra            |  among
contro         |  against
io             |  I
tu             |  thou
lui            |  he
lei            |  she
noi            |  we
voi            |  you
loro           |  they
mio            |  my
mia            |
miei           |
mie            |
tuo            |
tua            |
tuoi           |  thy
tue            |
suo            |
sua            |
suoi           |  his, her
sue            |
nostro         |  our
nostra         |
nostri         |
nostre         |
vostro         |  your
vostra         |
vostri         |
vostre         |
mi             |  me
ti             |  thee
ci             |  us, there
vi             |  you, there
lo             |  him, the
la             |  her, the
li             |  them
le             |  them, the
gli            |  to him, the
ne             |  from there etc
il             |  the
un             |  a
uno            |  a
una            |  a
ma             |  but
ed             |  and
se             |  if
perché         |  why, because
anche          |  also
come           |  how
dov            |  where (as dov')
dove           |  where
che            |  who, that
chi            |  who
cui            |  whom
non            |  not
più            |  more
quale          |  who, that
quanto         |  how much
quanti         |
quanta         |
quante         |
quello         |  that
quelli         |
quella         |
quelle         |
questo         |  this
questi         |
questa         |
queste         |
si             |  yes
tutto          |  all
tutti          |  all

               |  single letter forms:

a              |  at
c              |  as c' for ce or ci
e              |  and
i              |  the
l              |  as l'
o              |  or

               | forms of avere, to have (not including the infinitive):

ho
hai
ha
abbiamo
avete
hanno
abbia
abbiate
abbiano
avrò
avrai
avrà
avremo
avrete
avranno
avrei
avresti
avrebbe
avremmo
avreste
avrebbero
avevo
avevi
aveva
avevamo
avevate
avevano
ebbi
avesti
ebbe
avemmo
aveste
ebbero
avessi
avesse
avessimo
avessero
avendo
avuto
avuta
avuti
avute

               | forms of essere, to be (not including the infinitive):
sono
sei
è
siamo
siete
sia
siate
siano
sarò
sarai
sarà
saremo
sarete
saranno
sarei
saresti
sarebbe
saremmo
sareste
sarebbero
ero
eri
era
eravamo
eravate
erano
fui
fosti
fu
fummo
foste
furono
fossi
fosse
fossimo
fossero
essendo

               | forms of fare, to do (not including the infinitive, fa, fat-):
faccio
fai
facciamo
fanno
faccia
facciate
facciano
farò
farai
farà
faremo
farete
faranno
farei
faresti
farebbe
faremmo
fareste
farebbero
facevo
facevi
faceva
facevamo
facevate
facevano
feci
facesti
fece
facemmo
faceste
fecero
facessi
facesse
facessimo
facessero
facendo

               | forms of stare, to be (not including the infinitive):
sto
stai
sta
stiamo
stanno
stia
stiate
stiano
starò
starai
starà
staremo
starete
staranno
starei
staresti
starebbe
staremmo
stareste
starebbero
stavo
stavi
stava
stavamo
stavate
stavano
stetti
stesti
stette
stemmo
steste
stettero
stessi
stesse
stessimo
stessero
stando
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(ItalianStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
