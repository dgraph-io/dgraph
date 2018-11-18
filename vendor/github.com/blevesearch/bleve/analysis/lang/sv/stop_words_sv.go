package sv

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_sv"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/snowball/
// ` was changed to ' to allow for literal string

var SwedishStopWords = []byte(` | From svn.tartarus.org/snowball/trunk/website/algorithms/swedish/stop.txt
 | This file is distributed under the BSD License.
 | See http://snowball.tartarus.org/license.php
 | Also see http://www.opensource.org/licenses/bsd-license.html
 |  - Encoding was converted to UTF-8.
 |  - This notice was added.
 |
 | NOTE: To use this file with StopFilterFactory, you must specify format="snowball"

 | A Swedish stop word list. Comments begin with vertical bar. Each stop
 | word is at the start of a line.

 | This is a ranked list (commonest to rarest) of stopwords derived from
 | a large text sample.

 | Swedish stop words occasionally exhibit homonym clashes. For example
 |  så = so, but also seed. These are indicated clearly below.

och            | and
det            | it, this/that
att            | to (with infinitive)
i              | in, at
en             | a
jag            | I
hon            | she
som            | who, that
han            | he
på             | on
den            | it, this/that
med            | with
var            | where, each
sig            | him(self) etc
för            | for
så             | so (also: seed)
till           | to
är             | is
men            | but
ett            | a
om             | if; around, about
hade           | had
de             | they, these/those
av             | of
icke           | not, no
mig            | me
du             | you
henne          | her
då             | then, when
sin            | his
nu             | now
har            | have
inte           | inte någon = no one
hans           | his
honom          | him
skulle         | 'sake'
hennes         | her
där            | there
min            | my
man            | one (pronoun)
ej             | nor
vid            | at, by, on (also: vast)
kunde          | could
något          | some etc
från           | from, off
ut             | out
när            | when
efter          | after, behind
upp            | up
vi             | we
dem            | them
vara           | be
vad            | what
över           | over
än             | than
dig            | you
kan            | can
sina           | his
här            | here
ha             | have
mot            | towards
alla           | all
under          | under (also: wonder)
någon          | some etc
eller          | or (else)
allt           | all
mycket         | much
sedan          | since
ju             | why
denna          | this/that
själv          | myself, yourself etc
detta          | this/that
åt             | to
utan           | without
varit          | was
hur            | how
ingen          | no
mitt           | my
ni             | you
bli            | to be, become
blev           | from bli
oss            | us
din            | thy
dessa          | these/those
några          | some etc
deras          | their
blir           | from bli
mina           | my
samma          | (the) same
vilken         | who, that
er             | you, your
sådan          | such a
vår            | our
blivit         | from bli
dess           | its
inom           | within
mellan         | between
sådant         | such a
varför         | why
varje          | each
vilka          | who, that
ditt           | thy
vem            | who
vilket         | who, that
sitta          | his
sådana         | such a
vart           | each
dina           | thy
vars           | whose
vårt           | our
våra           | our
ert            | your
era            | your
vilkas         | whose

`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(SwedishStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
