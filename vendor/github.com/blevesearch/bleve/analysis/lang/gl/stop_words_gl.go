package gl

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_gl"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/
// ` was changed to ' to allow for literal string

var GalicianStopWords = []byte(`# galican stopwords
a
aínda
alí
aquel
aquela
aquelas
aqueles
aquilo
aquí
ao
aos
as
así
á
ben
cando
che
co
coa
comigo
con
connosco
contigo
convosco
coas
cos
cun
cuns
cunha
cunhas
da
dalgunha
dalgunhas
dalgún
dalgúns
das
de
del
dela
delas
deles
desde
deste
do
dos
dun
duns
dunha
dunhas
e
el
ela
elas
eles
en
era
eran
esa
esas
ese
eses
esta
estar
estaba
está
están
este
estes
estiven
estou
eu
é
facer
foi
foron
fun
había
hai
iso
isto
la
las
lle
lles
lo
los
mais
me
meu
meus
min
miña
miñas
moi
na
nas
neste
nin
no
non
nos
nosa
nosas
noso
nosos
nós
nun
nunha
nuns
nunhas
o
os
ou
ó
ós
para
pero
pode
pois
pola
polas
polo
polos
por
que
se
senón
ser
seu
seus
sexa
sido
sobre
súa
súas
tamén
tan
te
ten
teñen
teño
ter
teu
teus
ti
tido
tiña
tiven
túa
túas
un
unha
unhas
uns
vos
vosa
vosas
voso
vosos
vós
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(GalicianStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
