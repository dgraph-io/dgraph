package el

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_el"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/
// ` was changed to ' to allow for literal string

var GreekStopWords = []byte(`# Lucene Greek Stopwords list
# Note: by default this file is used after GreekLowerCaseFilter,
# so when modifying this file use 'σ' instead of 'ς' 
ο
η
το
οι
τα
του
τησ
των
τον
την
και 
κι
κ
ειμαι
εισαι
ειναι
ειμαστε
ειστε
στο
στον
στη
στην
μα
αλλα
απο
για
προσ
με
σε
ωσ
παρα
αντι
κατα
μετα
θα
να
δε
δεν
μη
μην
επι
ενω
εαν
αν
τοτε
που
πωσ
ποιοσ
ποια
ποιο
ποιοι
ποιεσ
ποιων
ποιουσ
αυτοσ
αυτη
αυτο
αυτοι
αυτων
αυτουσ
αυτεσ
αυτα
εκεινοσ
εκεινη
εκεινο
εκεινοι
εκεινεσ
εκεινα
εκεινων
εκεινουσ
οπωσ
ομωσ
ισωσ
οσο
οτι
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(GreekStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
