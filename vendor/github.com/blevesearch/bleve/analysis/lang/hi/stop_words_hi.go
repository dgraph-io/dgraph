package hi

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_hi"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/
// ` was changed to ' to allow for literal string

var HindiStopWords = []byte(`# Also see http://www.opensource.org/licenses/bsd-license.html
# See http://members.unine.ch/jacques.savoy/clef/index.html.
# This file was created by Jacques Savoy and is distributed under the BSD license.
# Note: by default this file also contains forms normalized by HindiNormalizer 
# for spelling variation (see section below), such that it can be used whether or 
# not you enable that feature. When adding additional entries to this list,
# please add the normalized form as well. 
अंदर
अत
अपना
अपनी
अपने
अभी
आदि
आप
इत्यादि
इन 
इनका
इन्हीं
इन्हें
इन्हों
इस
इसका
इसकी
इसके
इसमें
इसी
इसे
उन
उनका
उनकी
उनके
उनको
उन्हीं
उन्हें
उन्हों
उस
उसके
उसी
उसे
एक
एवं
एस
ऐसे
और
कई
कर
करता
करते
करना
करने
करें
कहते
कहा
का
काफ़ी
कि
कितना
किन्हें
किन्हों
किया
किर
किस
किसी
किसे
की
कुछ
कुल
के
को
कोई
कौन
कौनसा
गया
घर
जब
जहाँ
जा
जितना
जिन
जिन्हें
जिन्हों
जिस
जिसे
जीधर
जैसा
जैसे
जो
तक
तब
तरह
तिन
तिन्हें
तिन्हों
तिस
तिसे
तो
था
थी
थे
दबारा
दिया
दुसरा
दूसरे
दो
द्वारा
न
नहीं
ना
निहायत
नीचे
ने
पर
पर  
पहले
पूरा
पे
फिर
बनी
बही
बहुत
बाद
बाला
बिलकुल
भी
भीतर
मगर
मानो
मे
में
यदि
यह
यहाँ
यही
या
यिह 
ये
रखें
रहा
रहे
ऱ्वासा
लिए
लिये
लेकिन
व
वर्ग
वह
वह 
वहाँ
वहीं
वाले
वुह 
वे
वग़ैरह
संग
सकता
सकते
सबसे
सभी
साथ
साबुत
साभ
सारा
से
सो
ही
हुआ
हुई
हुए
है
हैं
हो
होता
होती
होते
होना
होने
# additional normalized forms of the above
अपनि
जेसे
होति
सभि
तिंहों
इंहों
दवारा
इसि
किंहें
थि
उंहों
ओर
जिंहें
वहिं
अभि
बनि
हि
उंहिं
उंहें
हें
वगेरह
एसे
रवासा
कोन
निचे
काफि
उसि
पुरा
भितर
हे
बहि
वहां
कोइ
यहां
जिंहों
तिंहें
किसि
कइ
यहि
इंहिं
जिधर
इंहें
अदि
इतयादि
हुइ
कोनसा
इसकि
दुसरे
जहां
अप
किंहों
उनकि
भि
वरग
हुअ
जेसा
नहिं
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(HindiStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
