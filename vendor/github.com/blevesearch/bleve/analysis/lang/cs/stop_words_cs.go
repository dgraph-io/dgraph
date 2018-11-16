package cs

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_cs"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/
// ` was changed to ' to allow for literal string

var CzechStopWords = []byte(`a
s
k
o
i
u
v
z
dnes
cz
tímto
budeš
budem
byli
jseš
můj
svým
ta
tomto
tohle
tuto
tyto
jej
zda
proč
máte
tato
kam
tohoto
kdo
kteří
mi
nám
tom
tomuto
mít
nic
proto
kterou
byla
toho
protože
asi
ho
naši
napište
re
což
tím
takže
svých
její
svými
jste
aj
tu
tedy
teto
bylo
kde
ke
pravé
ji
nad
nejsou
či
pod
téma
mezi
přes
ty
pak
vám
ani
když
však
neg
jsem
tento
článku
články
aby
jsme
před
pta
jejich
byl
ještě
až
bez
také
pouze
první
vaše
která
nás
nový
tipy
pokud
může
strana
jeho
své
jiné
zprávy
nové
není
vás
jen
podle
zde
už
být
více
bude
již
než
který
by
které
co
nebo
ten
tak
má
při
od
po
jsou
jak
další
ale
si
se
ve
to
jako
za
zpět
ze
do
pro
je
na
atd
atp
jakmile
přičemž
já
on
ona
ono
oni
ony
my
vy
jí
ji
mě
mne
jemu
tomu
těm
těmu
němu
němuž
jehož
jíž
jelikož
jež
jakož
načež
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(CzechStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
