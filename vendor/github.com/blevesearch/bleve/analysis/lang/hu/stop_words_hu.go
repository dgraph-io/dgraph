package hu

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_hu"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/snowball/
// ` was changed to ' to allow for literal string

var HungarianStopWords = []byte(` | From svn.tartarus.org/snowball/trunk/website/algorithms/hungarian/stop.txt
 | This file is distributed under the BSD License.
 | See http://snowball.tartarus.org/license.php
 | Also see http://www.opensource.org/licenses/bsd-license.html
 |  - Encoding was converted to UTF-8.
 |  - This notice was added.
 |
 | NOTE: To use this file with StopFilterFactory, you must specify format="snowball"
 
| Hungarian stop word list
| prepared by Anna Tordai

a
ahogy
ahol
aki
akik
akkor
alatt
által
általában
amely
amelyek
amelyekben
amelyeket
amelyet
amelynek
ami
amit
amolyan
amíg
amikor
át
abban
ahhoz
annak
arra
arról
az
azok
azon
azt
azzal
azért
aztán
azután
azonban
bár
be
belül
benne
cikk
cikkek
cikkeket
csak
de
e
eddig
egész
egy
egyes
egyetlen
egyéb
egyik
egyre
ekkor
el
elég
ellen
elő
először
előtt
első
én
éppen
ebben
ehhez
emilyen
ennek
erre
ez
ezt
ezek
ezen
ezzel
ezért
és
fel
felé
hanem
hiszen
hogy
hogyan
igen
így
illetve
ill.
ill
ilyen
ilyenkor
ison
ismét
itt
jó
jól
jobban
kell
kellett
keresztül
keressünk
ki
kívül
között
közül
legalább
lehet
lehetett
legyen
lenne
lenni
lesz
lett
maga
magát
majd
majd
már
más
másik
meg
még
mellett
mert
mely
melyek
mi
mit
míg
miért
milyen
mikor
minden
mindent
mindenki
mindig
mint
mintha
mivel
most
nagy
nagyobb
nagyon
ne
néha
nekem
neki
nem
néhány
nélkül
nincs
olyan
ott
össze
ő
ők
őket
pedig
persze
rá
s
saját
sem
semmi
sok
sokat
sokkal
számára
szemben
szerint
szinte
talán
tehát
teljes
tovább
továbbá
több
úgy
ugyanis
új
újabb
újra
után
utána
utolsó
vagy
vagyis
valaki
valami
valamint
való
vagyok
van
vannak
volt
voltam
voltak
voltunk
vissza
vele
viszont
volna
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(HungarianStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
