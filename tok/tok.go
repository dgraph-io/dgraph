/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package tok

import (
	"encoding/binary"
	"time"

	farm "github.com/dgryski/go-farm"
	geom "github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/yanyiwu/gojieba"
)

// Tokenizer defines what a tokenizer must provide.
type Tokenizer interface {

	// Name is name of tokenizer. This should be unique.
	Name() string

	// Type returns typeID that we care about.
	Type() types.TypeID

	// Tokens return tokens for a given value.
	Tokens(sv types.Val, indexing bool) ([]string, error)

	// Identifier returns the prefix byte for this token type.
	Identifier() byte

	// IsSortable returns true if the tokenizer can be used for sorting/ordering.
	IsSortable() bool

	// IsLossy() returns true if we don't store the values directly as index keys
	// during tokenization. If a predicate is tokenized using an IsLossy() tokenizer,
	// then we need to fetch the actual value and compare.
	IsLossy() bool
}

var (
	tokenizers map[string]Tokenizer
	defaults   map[types.TypeID]Tokenizer
	jiebaSearch *gojieba.Jieba
)

func init() {
	RegisterTokenizer(GeoTokenizer{})
	RegisterTokenizer(IntTokenizer{})
	RegisterTokenizer(FloatTokenizer{})
	RegisterTokenizer(YearTokenizer{})
	RegisterTokenizer(HourTokenizer{})
	RegisterTokenizer(MonthTokenizer{})
	RegisterTokenizer(DayTokenizer{})
	RegisterTokenizer(TermTokenizer{})
	RegisterTokenizer(ExactTokenizer{})
	RegisterTokenizer(BoolTokenizer{})
	RegisterTokenizer(TrigramTokenizer{})
	RegisterTokenizer(HashTokenizer{})

	// Check for duplicate prefix bytes.
	usedIds := make(map[byte]struct{})
	for _, tok := range tokenizers {
		tokID := tok.Identifier()
		_, ok := usedIds[tokID]
		x.AssertTruef(!ok, "Same ID used by multiple tokenizers: %#x", tokID)
		usedIds[tokID] = struct{}{}
	}

	// all full-text tokenizers share the same Identifier, so we skip the test
	initFullTextTokenizers()
}

// GetTokenizer returns tokenizer given unique name.
func GetTokenizer(name string) (Tokenizer, bool) {
	t, found := tokenizers[name]
	return t, found
}

// RegisterTokenizer adds your tokenizer to our list.
func RegisterTokenizer(t Tokenizer) {
	if tokenizers == nil {
		tokenizers = make(map[string]Tokenizer)
	}
	name := t.Name()
	_, found := tokenizers[name]
	x.AssertTruef(!found, "Duplicate tokenizer name %s", name)
	tokenizers[name] = t
}

type GeoTokenizer struct{}

func (t GeoTokenizer) Name() string       { return "geo" }
func (t GeoTokenizer) Type() types.TypeID { return types.GeoID }
func (t GeoTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	tokens, err := types.IndexGeoTokens(sv.Value.(geom.T))
	EncodeGeoTokens(tokens)
	return tokens, err
}
func (t GeoTokenizer) Identifier() byte { return 0x5 }
func (t GeoTokenizer) IsSortable() bool { return false }
func (t GeoTokenizer) IsLossy() bool    { return true }

type IntTokenizer struct{}

func (t IntTokenizer) Name() string       { return "int" }
func (t IntTokenizer) Type() types.TypeID { return types.IntID }
func (t IntTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	return []string{encodeToken(encodeInt(sv.Value.(int64)), t.Identifier())}, nil
}
func (t IntTokenizer) Identifier() byte { return 0x6 }
func (t IntTokenizer) IsSortable() bool { return true }
func (t IntTokenizer) IsLossy() bool    { return false }

type FloatTokenizer struct{}

func (t FloatTokenizer) Name() string       { return "float" }
func (t FloatTokenizer) Type() types.TypeID { return types.FloatID }
func (t FloatTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	return []string{encodeToken(encodeInt(int64(sv.Value.(float64))), t.Identifier())}, nil
}
func (t FloatTokenizer) Identifier() byte { return 0x7 }
func (t FloatTokenizer) IsSortable() bool { return true }
func (t FloatTokenizer) IsLossy() bool    { return true }

type YearTokenizer struct{}

func (t YearTokenizer) Name() string       { return "year" }
func (t YearTokenizer) Type() types.TypeID { return types.DateTimeID }
func (t YearTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	tval := sv.Value.(time.Time)
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.Year()))
	return []string{encodeToken(string(buf), t.Identifier())}, nil
}
func (t YearTokenizer) Identifier() byte { return 0x4 }
func (t YearTokenizer) IsSortable() bool { return true }
func (t YearTokenizer) IsLossy() bool    { return true }

type MonthTokenizer struct{}

func (t MonthTokenizer) Name() string       { return "month" }
func (t MonthTokenizer) Type() types.TypeID { return types.DateTimeID }
func (t MonthTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	tval := sv.Value.(time.Time)
	buf := make([]byte, 4)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.Year()))
	binary.BigEndian.PutUint16(buf[2:4], uint16(tval.Month()))
	return []string{encodeToken(string(buf), t.Identifier())}, nil
}
func (t MonthTokenizer) Identifier() byte { return 0x41 }
func (t MonthTokenizer) IsSortable() bool { return true }
func (t MonthTokenizer) IsLossy() bool    { return true }

type DayTokenizer struct{}

func (t DayTokenizer) Name() string       { return "day" }
func (t DayTokenizer) Type() types.TypeID { return types.DateTimeID }
func (t DayTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	tval := sv.Value.(time.Time)
	buf := make([]byte, 6)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.Year()))
	binary.BigEndian.PutUint16(buf[2:4], uint16(tval.Month()))
	binary.BigEndian.PutUint16(buf[4:6], uint16(tval.Day()))
	return []string{encodeToken(string(buf), t.Identifier())}, nil
}
func (t DayTokenizer) Identifier() byte { return 0x42 }
func (t DayTokenizer) IsSortable() bool { return true }
func (t DayTokenizer) IsLossy() bool    { return true }

type HourTokenizer struct{}

func (t HourTokenizer) Name() string       { return "hour" }
func (t HourTokenizer) Type() types.TypeID { return types.DateTimeID }
func (t HourTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	tval := sv.Value.(time.Time)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.Year()))
	binary.BigEndian.PutUint16(buf[2:4], uint16(tval.Month()))
	binary.BigEndian.PutUint16(buf[4:6], uint16(tval.Day()))
	binary.BigEndian.PutUint16(buf[6:8], uint16(tval.Hour()))
	return []string{encodeToken(string(buf), t.Identifier())}, nil
}
func (t HourTokenizer) Identifier() byte { return 0x43 }
func (t HourTokenizer) IsSortable() bool { return true }
func (t HourTokenizer) IsLossy() bool    { return true }

type TermTokenizer struct{}

func (t TermTokenizer) Name() string       { return "term" }
func (t TermTokenizer) Type() types.TypeID { return types.StringID }
func (t TermTokenizer) Tokens(sv types.Val, indexing bool) ([]string, error) {
	return getBleveTokens(t.Name(), t.Identifier(), sv, indexing)
}
func (t TermTokenizer) Identifier() byte { return 0x1 }
func (t TermTokenizer) IsSortable() bool { return false }
func (t TermTokenizer) IsLossy() bool    { return true }

type ExactTokenizer struct{}

func (t ExactTokenizer) Name() string       { return "exact" }
func (t ExactTokenizer) Type() types.TypeID { return types.StringID }
func (t ExactTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	term, ok := sv.Value.(string)
	if !ok {
		return nil, x.Errorf("Exact indices only supported for string types")
	}
	if len(term) > 100 {
		x.Printf("Long text for exact index. Consider switching to hash for better performance\n")
	}
	return []string{encodeToken(term, t.Identifier())}, nil
}
func (t ExactTokenizer) Identifier() byte { return 0x2 }
func (t ExactTokenizer) IsSortable() bool { return true }
func (t ExactTokenizer) IsLossy() bool    { return false }

// Full text tokenizer, with language support
type FullTextTokenizer struct {
	Lang string
}

func (t FullTextTokenizer) Name() string       { return FtsTokenizerName(t.Lang) }
func (t FullTextTokenizer) Type() types.TypeID { return types.StringID }
func (t FullTextTokenizer) Tokens(sv types.Val, indexing bool) ([]string, error) {
	return getBleveTokens(t.Name(), t.Identifier(), sv, indexing)
}
func (t FullTextTokenizer) Identifier() byte { return 0x8 }
func (t FullTextTokenizer) IsSortable() bool { return false }
func (t FullTextTokenizer) IsLossy() bool    { return true }

func getBleveTokens(name string, identifier byte, sv types.Val, indexing bool) ([]string, error) {
	if name==FtsTokenizerName("zh-hans") && !indexing {
		// Seems Gojieba/bleve doesn't have an exact mode, use Gojieba directly
		// ....
		words := jiebaSearch.Cut(sv.Value.(string), true)
		terms := make([]string, len(words))
		for i, token := range words {
			terms[i] = encodeToken(token, identifier)
		}
		terms = x.RemoveDuplicates(terms)
		return terms, nil
	}
	analyzer, err := bleveCache.AnalyzerNamed(name)
	if err != nil {
		return nil, err
	}
	tokenStream := analyzer.Analyze([]byte(sv.Value.(string)))

	terms := make([]string, len(tokenStream))
	for i, token := range tokenStream {
		terms[i] = encodeToken(string(token.Term), identifier)
	}
	terms = x.RemoveDuplicates(terms)
	return terms, nil
}

func encodeInt(val int64) string {
	buf := make([]byte, 9)
	binary.BigEndian.PutUint64(buf[1:], uint64(val))
	if val < 0 {
		buf[0] = 0
	} else {
		buf[0] = 1
	}
	return string(buf)
}

func encodeToken(tok string, typ byte) string {
	return string(typ) + tok
}

func EncodeGeoTokens(tokens []string) {
	for i := 0; i < len(tokens); i++ {
		tokens[i] = encodeToken(tokens[i], GeoTokenizer{}.Identifier())
	}
}

func EncodeRegexTokens(tokens []string) {
	for i := 0; i < len(tokens); i++ {
		tokens[i] = encodeToken(tokens[i], TrigramTokenizer{}.Identifier())
	}
}

type BoolTokenizer struct{}

func (t BoolTokenizer) Name() string       { return "bool" }
func (t BoolTokenizer) Type() types.TypeID { return types.BoolID }
func (t BoolTokenizer) Tokens(v types.Val, _ bool) ([]string, error) {
	var b int64
	if v.Value.(bool) {
		b = 1
	}
	return []string{encodeToken(encodeInt(b), t.Identifier())}, nil
}
func (t BoolTokenizer) Identifier() byte { return 0x9 }
func (t BoolTokenizer) IsSortable() bool { return false }
func (t BoolTokenizer) IsLossy() bool    { return false }

type TrigramTokenizer struct{}

func (t TrigramTokenizer) Name() string       { return "trigram" }
func (t TrigramTokenizer) Type() types.TypeID { return types.StringID }
func (t TrigramTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	value, ok := sv.Value.(string)
	if !ok {
		return nil, x.Errorf("Trigram indices only supported for string types")
	}
	l := len(value) - 2
	if l > 0 {
		tokens := make([]string, l)
		for i := 0; i < l; i++ {
			trigram := value[i : i+3]
			tokens[i] = encodeToken(trigram, t.Identifier())
		}
		tokens = x.RemoveDuplicates(tokens)
		return tokens, nil
	}
	return nil, nil
}
func (t TrigramTokenizer) Identifier() byte { return 0xA }
func (t TrigramTokenizer) IsSortable() bool { return false }
func (t TrigramTokenizer) IsLossy() bool    { return true }

type HashTokenizer struct{}

func (t HashTokenizer) Name() string       { return "hash" }
func (t HashTokenizer) Type() types.TypeID { return types.StringID }
func (t HashTokenizer) Tokens(sv types.Val, _ bool) ([]string, error) {
	term, ok := sv.Value.(string)
	if !ok {
		return nil, x.Errorf("Hash tokenizer only supported for string types")
	}
	var hash [8]byte
	binary.BigEndian.PutUint64(hash[:], farm.Hash64([]byte(term)))
	return []string{encodeToken(string(hash[:]), t.Identifier())}, nil
}
func (t HashTokenizer) Identifier() byte { return 0xB }
func (t HashTokenizer) IsSortable() bool { return false }
func (t HashTokenizer) IsLossy() bool    { return true }

func init() {
	jiebaSearch = gojieba.NewJieba(
		gojieba.DICT_PATH,
		gojieba.HMM_PATH,
		gojieba.USER_DICT_PATH,
		gojieba.IDF_PATH,
		gojieba.STOP_WORDS_PATH,
	)
}