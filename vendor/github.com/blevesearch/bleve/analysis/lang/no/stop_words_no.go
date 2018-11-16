package no

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_no"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/snowball/
// ` was changed to ' to allow for literal string

var NorwegianStopWords = []byte(` | From svn.tartarus.org/snowball/trunk/website/algorithms/norwegian/stop.txt
 | This file is distributed under the BSD License.
 | See http://snowball.tartarus.org/license.php
 | Also see http://www.opensource.org/licenses/bsd-license.html
 |  - Encoding was converted to UTF-8.
 |  - This notice was added.
 |
 | NOTE: To use this file with StopFilterFactory, you must specify format="snowball"

 | A Norwegian stop word list. Comments begin with vertical bar. Each stop
 | word is at the start of a line.

 | This stop word list is for the dominant bokmål dialect. Words unique
 | to nynorsk are marked *.

 | Revised by Jan Bruusgaard <Jan.Bruusgaard@ssb.no>, Jan 2005

og             | and
i              | in
jeg            | I
det            | it/this/that
at             | to (w. inf.)
en             | a/an
et             | a/an
den            | it/this/that
til            | to
er             | is/am/are
som            | who/that
på             | on
de             | they / you(formal)
med            | with
han            | he
av             | of
ikke           | not
ikkje          | not *
der            | there
så             | so
var            | was/were
meg            | me
seg            | you
men            | but
ett            | one
har            | have
om             | about
vi             | we
min            | my
mitt           | my
ha             | have
hadde          | had
hun            | she
nå             | now
over           | over
da             | when/as
ved            | by/know
fra            | from
du             | you
ut             | out
sin            | your
dem            | them
oss            | us
opp            | up
man            | you/one
kan            | can
hans           | his
hvor           | where
eller          | or
hva            | what
skal           | shall/must
selv           | self (reflective)
sjøl           | self (reflective)
her            | here
alle           | all
vil            | will
bli            | become
ble            | became
blei           | became *
blitt          | have become
kunne          | could
inn            | in
når            | when
være           | be
kom            | come
noen           | some
noe            | some
ville          | would
dere           | you
som            | who/which/that
deres          | their/theirs
kun            | only/just
ja             | yes
etter          | after
ned            | down
skulle         | should
denne          | this
for            | for/because
deg            | you
si             | hers/his
sine           | hers/his
sitt           | hers/his
mot            | against
å              | to
meget          | much
hvorfor        | why
dette          | this
disse          | these/those
uten           | without
hvordan        | how
ingen          | none
din            | your
ditt           | your
blir           | become
samme          | same
hvilken        | which
hvilke         | which (plural)
sånn           | such a
inni           | inside/within
mellom         | between
vår            | our
hver           | each
hvem           | who
vors           | us/ours
hvis           | whose
både           | both
bare           | only/just
enn            | than
fordi          | as/because
før            | before
mange          | many
også           | also
slik           | just
vært           | been
være           | to be
båe            | both *
begge          | both
siden          | since
dykk           | your *
dykkar         | yours *
dei            | they *
deira          | them *
deires         | theirs *
deim           | them *
di             | your (fem.) *
då             | as/when *
eg             | I *
ein            | a/an *
eit            | a/an *
eitt           | a/an *
elles          | or *
honom          | he *
hjå            | at *
ho             | she *
hoe            | she *
henne          | her
hennar         | her/hers
hennes         | hers
hoss           | how *
hossen         | how *
ikkje          | not *
ingi           | noone *
inkje          | noone *
korleis        | how *
korso          | how *
kva            | what/which *
kvar           | where *
kvarhelst      | where *
kven           | who/whom *
kvi            | why *
kvifor         | why *
me             | we *
medan          | while *
mi             | my *
mine           | my *
mykje          | much *
no             | now *
nokon          | some (masc./neut.) *
noka           | some (fem.) *
nokor          | some *
noko           | some *
nokre          | some *
si             | his/hers *
sia            | since *
sidan          | since *
so             | so *
somt           | some *
somme          | some *
um             | about*
upp            | up *
vere           | be *
vore           | was *
verte          | become *
vort           | become *
varte          | became *
vart           | became *

`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(NorwegianStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
