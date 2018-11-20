package bg

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_bg"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/
// ` was changed to ' to allow for literal string

var BulgarianStopWords = []byte(`# This file was created by Jacques Savoy and is distributed under the BSD license.
# See http://members.unine.ch/jacques.savoy/clef/index.html.
# Also see http://www.opensource.org/licenses/bsd-license.html
а
аз
ако
ала
бе
без
беше
би
бил
била
били
било
близо
бъдат
бъде
бяха
в
вас
ваш
ваша
вероятно
вече
взема
ви
вие
винаги
все
всеки
всички
всичко
всяка
във
въпреки
върху
г
ги
главно
го
д
да
дали
до
докато
докога
дори
досега
доста
е
едва
един
ето
за
зад
заедно
заради
засега
затова
защо
защото
и
из
или
им
има
имат
иска
й
каза
как
каква
какво
както
какъв
като
кога
когато
което
които
кой
който
колко
която
къде
където
към
ли
м
ме
между
мен
ми
мнозина
мога
могат
може
моля
момента
му
н
на
над
назад
най
направи
напред
например
нас
не
него
нея
ни
ние
никой
нито
но
някои
някой
няма
обаче
около
освен
особено
от
отгоре
отново
още
пак
по
повече
повечето
под
поне
поради
после
почти
прави
пред
преди
през
при
пък
първо
с
са
само
се
сега
си
скоро
след
сме
според
сред
срещу
сте
съм
със
също
т
тази
така
такива
такъв
там
твой
те
тези
ти
тн
то
това
тогава
този
той
толкова
точно
трябва
тук
тъй
тя
тях
у
харесва
ч
че
често
чрез
ще
щом
я
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(BulgarianStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
