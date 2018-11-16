package eu

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_eu"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/
// ` was changed to ' to allow for literal string

var BasqueStopWords = []byte(`# example set of basque stopwords
al
anitz
arabera
asko
baina
bat
batean
batek
bati
batzuei
batzuek
batzuetan
batzuk
bera
beraiek
berau
berauek
bere
berori
beroriek
beste
bezala
da
dago
dira
ditu
du
dute
edo
egin
ere
eta
eurak
ez
gainera
gu
gutxi
guzti
haiei
haiek
haietan
hainbeste
hala
han
handik
hango
hara
hari
hark
hartan
hau
hauei
hauek
hauetan
hemen
hemendik
hemengo
hi
hona
honek
honela
honetan
honi
hor
hori
horiei
horiek
horietan
horko
horra
horrek
horrela
horretan
horri
hortik
hura
izan
ni
noiz
nola
non
nondik
nongo
nor
nora
ze
zein
zen
zenbait
zenbat
zer
zergatik
ziren
zituen
zu
zuek
zuen
zuten
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(BasqueStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
