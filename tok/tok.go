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
	"plugin"
	"strings"
	"time"

	farm "github.com/dgryski/go-farm"
	geom "github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// Tokenizer defines what a tokenizer must provide.
type Tokenizer interface {

	// Name is name of tokenizer. This should be unique.
	Name() string

	// Type returns the string representation of the typeID that we care about.
	Type() string

	// Tokens return tokens for a given value. The tokens shouldn't be encoded
	// with the byte identifier.
	Tokens(value interface{}) ([]string, error)

	// Identifier returns the prefix byte for this token type. This should be
	// unique. The range 0x80 to 0xff (inclusive) is reserved for user-provided
	// custom tokenizers.
	Identifier() byte

	// IsSortable returns true if the tokenizer can be used for sorting/ordering.
	IsSortable() bool

	// IsLossy() returns true if we don't store the values directly as index keys
	// during tokenization. If a predicate is tokenized using an IsLossy() tokenizer,
	// then we need to fetch the actual value and compare.
	IsLossy() bool
}

// BuildTokens tokenizes a value, creating strings that can be used to create
// index keys.
func BuildTokens(val interface{}, t Tokenizer) ([]string, error) {
	tokens, err := t.Tokens(val)
	if err != nil {
		return nil, err
	}
	id := t.Identifier()
	for i := range tokens {
		tokens[i] = encodeToken(tokens[i], id)
	}
	return tokens, nil
}

var tokenizers map[string]Tokenizer

func init() {
	registerTokenizer(GeoTokenizer{})
	registerTokenizer(IntTokenizer{})
	registerTokenizer(FloatTokenizer{})
	registerTokenizer(YearTokenizer{})
	registerTokenizer(HourTokenizer{})
	registerTokenizer(MonthTokenizer{})
	registerTokenizer(DayTokenizer{})
	registerTokenizer(TermTokenizer{})
	registerTokenizer(ExactTokenizer{})
	registerTokenizer(BoolTokenizer{})
	registerTokenizer(TrigramTokenizer{})
	registerTokenizer(HashTokenizer{})
	initFullTextTokenizers()
}

func LoadCustomTokenizer(soFile string) {
	x.Printf("Loading custom tokenizer from %q", soFile)
	pl, err := plugin.Open(soFile)
	x.Checkf(err, "could not open custom tokenizer plugin file")
	symb, err := pl.Lookup("Tokenizer")
	x.Checkf(err, "could not find symbol \"Tokenizer\" "+
		"while loading custom tokenizer: %v", err)

	// Let any type assertion panics occur, since they will contain a message
	// telling the user what went wrong. Otherwise it's hard to capture this
	// information to pass on to the user.
	tokenizer := symb.(func() interface{})().(Tokenizer)

	id := tokenizer.Identifier()
	x.AssertTruef(id >= 0x80,
		"custom tokenizer identifier byte must be >= 0x80, but was %#x", id)
	registerTokenizer(tokenizer)
}

// GetTokenizer returns tokenizer given unique name.
func GetTokenizer(name string) (Tokenizer, bool) {
	t, found := tokenizers[name]
	return t, found
}

func registerTokenizer(t Tokenizer) {
	if tokenizers == nil {
		tokenizers = make(map[string]Tokenizer)
	}
	name := t.Name()
	_, found := tokenizers[name]
	x.AssertTruef(!found, "Duplicate tokenizer name %s", name)
	for _, tok := range tokenizers {
		// all full-text tokenizers share the same Identifier, so they skip the test
		isFTS := strings.HasPrefix(tok.Name(), FTSTokenizerName)
		x.AssertTruef(isFTS || tok.Identifier() != t.Identifier(),
			"Duplicate tokenizer id byte %#x, %s and %s", tok.Identifier(), tok.Name(), t.Name())
	}
	_, validType := types.TypeForName(t.Type())
	x.AssertTruef(validType, "t.Type() returned %q which isn't a valid type name")
	tokenizers[name] = t
}

type GeoTokenizer struct{}

func (t GeoTokenizer) Name() string { return "geo" }
func (t GeoTokenizer) Type() string { return "geo" }
func (t GeoTokenizer) Tokens(v interface{}) ([]string, error) {
	return types.IndexGeoTokens(v.(geom.T))
}
func (t GeoTokenizer) Identifier() byte { return 0x5 }
func (t GeoTokenizer) IsSortable() bool { return false }
func (t GeoTokenizer) IsLossy() bool    { return true }

type IntTokenizer struct{}

func (t IntTokenizer) Name() string { return "int" }
func (t IntTokenizer) Type() string { return "int" }
func (t IntTokenizer) Tokens(v interface{}) ([]string, error) {
	return []string{encodeInt(v.(int64))}, nil
}
func (t IntTokenizer) Identifier() byte { return 0x6 }
func (t IntTokenizer) IsSortable() bool { return true }
func (t IntTokenizer) IsLossy() bool    { return false }

type FloatTokenizer struct{}

func (t FloatTokenizer) Name() string { return "float" }
func (t FloatTokenizer) Type() string { return "float" }
func (t FloatTokenizer) Tokens(v interface{}) ([]string, error) {
	return []string{encodeInt(int64(v.(float64)))}, nil
}
func (t FloatTokenizer) Identifier() byte { return 0x7 }
func (t FloatTokenizer) IsSortable() bool { return true }
func (t FloatTokenizer) IsLossy() bool    { return true }

type YearTokenizer struct{}

func (t YearTokenizer) Name() string { return "year" }
func (t YearTokenizer) Type() string { return "datetime" }
func (t YearTokenizer) Tokens(v interface{}) ([]string, error) {
	tval := v.(time.Time)
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.Year()))
	return []string{string(buf)}, nil
}
func (t YearTokenizer) Identifier() byte { return 0x4 }
func (t YearTokenizer) IsSortable() bool { return true }
func (t YearTokenizer) IsLossy() bool    { return true }

type MonthTokenizer struct{}

func (t MonthTokenizer) Name() string { return "month" }
func (t MonthTokenizer) Type() string { return "datetime" }
func (t MonthTokenizer) Tokens(v interface{}) ([]string, error) {
	tval := v.(time.Time)
	buf := make([]byte, 4)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.Year()))
	binary.BigEndian.PutUint16(buf[2:4], uint16(tval.Month()))
	return []string{string(buf)}, nil
}
func (t MonthTokenizer) Identifier() byte { return 0x41 }
func (t MonthTokenizer) IsSortable() bool { return true }
func (t MonthTokenizer) IsLossy() bool    { return true }

type DayTokenizer struct{}

func (t DayTokenizer) Name() string { return "day" }
func (t DayTokenizer) Type() string { return "datetime" }
func (t DayTokenizer) Tokens(v interface{}) ([]string, error) {
	tval := v.(time.Time)
	buf := make([]byte, 6)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.Year()))
	binary.BigEndian.PutUint16(buf[2:4], uint16(tval.Month()))
	binary.BigEndian.PutUint16(buf[4:6], uint16(tval.Day()))
	return []string{string(buf)}, nil
}
func (t DayTokenizer) Identifier() byte { return 0x42 }
func (t DayTokenizer) IsSortable() bool { return true }
func (t DayTokenizer) IsLossy() bool    { return true }

type HourTokenizer struct{}

func (t HourTokenizer) Name() string { return "hour" }
func (t HourTokenizer) Type() string { return "datetime" }
func (t HourTokenizer) Tokens(v interface{}) ([]string, error) {
	tval := v.(time.Time)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.Year()))
	binary.BigEndian.PutUint16(buf[2:4], uint16(tval.Month()))
	binary.BigEndian.PutUint16(buf[4:6], uint16(tval.Day()))
	binary.BigEndian.PutUint16(buf[6:8], uint16(tval.Hour()))
	return []string{string(buf)}, nil
}
func (t HourTokenizer) Identifier() byte { return 0x43 }
func (t HourTokenizer) IsSortable() bool { return true }
func (t HourTokenizer) IsLossy() bool    { return true }

type TermTokenizer struct{}

func (t TermTokenizer) Name() string { return "term" }
func (t TermTokenizer) Type() string { return "string" }
func (t TermTokenizer) Tokens(v interface{}) ([]string, error) {
	return getBleveTokens(t.Name(), v.(string))
}
func (t TermTokenizer) Identifier() byte { return 0x1 }
func (t TermTokenizer) IsSortable() bool { return false }
func (t TermTokenizer) IsLossy() bool    { return true }

type ExactTokenizer struct{}

func (t ExactTokenizer) Name() string { return "exact" }
func (t ExactTokenizer) Type() string { return "string" }
func (t ExactTokenizer) Tokens(v interface{}) ([]string, error) {
	term, ok := v.(string)
	if !ok {
		return nil, x.Errorf("Exact indices only supported for string types")
	}
	if len(term) > 100 {
		x.Printf("Long text for exact index. Consider switching to hash for better performance\n")
	}
	return []string{term}, nil
}
func (t ExactTokenizer) Identifier() byte { return 0x2 }
func (t ExactTokenizer) IsSortable() bool { return true }
func (t ExactTokenizer) IsLossy() bool    { return false }

// Full text tokenizer, with language support
type FullTextTokenizer struct {
	Lang string
}

func (t FullTextTokenizer) Name() string { return FtsTokenizerName(t.Lang) }
func (t FullTextTokenizer) Type() string { return "string" }
func (t FullTextTokenizer) Tokens(v interface{}) ([]string, error) {
	return getBleveTokens(t.Name(), v.(string))
}
func (t FullTextTokenizer) Identifier() byte { return 0x8 }
func (t FullTextTokenizer) IsSortable() bool { return false }
func (t FullTextTokenizer) IsLossy() bool    { return true }

func getBleveTokens(name string, str string) ([]string, error) {
	analyzer, err := bleveCache.AnalyzerNamed(name)
	if err != nil {
		return nil, err
	}
	tokenStream := analyzer.Analyze([]byte(str))

	terms := make([]string, len(tokenStream))
	for i, token := range tokenStream {
		terms[i] = string(token.Term)
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

func (t BoolTokenizer) Name() string { return "bool" }
func (t BoolTokenizer) Type() string { return "bool" }
func (t BoolTokenizer) Tokens(v interface{}) ([]string, error) {
	var b int64
	if v.(bool) {
		b = 1
	}
	return []string{encodeInt(b)}, nil
}
func (t BoolTokenizer) Identifier() byte { return 0x9 }
func (t BoolTokenizer) IsSortable() bool { return false }
func (t BoolTokenizer) IsLossy() bool    { return false }

type TrigramTokenizer struct{}

func (t TrigramTokenizer) Name() string { return "trigram" }
func (t TrigramTokenizer) Type() string { return "string" }
func (t TrigramTokenizer) Tokens(v interface{}) ([]string, error) {
	value, ok := v.(string)
	if !ok {
		return nil, x.Errorf("Trigram indices only supported for string types")
	}
	l := len(value) - 2
	if l > 0 {
		tokens := make([]string, l)
		for i := 0; i < l; i++ {
			tokens[i] = value[i : i+3]
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

func (t HashTokenizer) Name() string { return "hash" }
func (t HashTokenizer) Type() string { return "string" }
func (t HashTokenizer) Tokens(v interface{}) ([]string, error) {
	term, ok := v.(string)
	if !ok {
		return nil, x.Errorf("Hash tokenizer only supported for string types")
	}
	var hash [8]byte
	binary.BigEndian.PutUint64(hash[:], farm.Hash64([]byte(term)))
	return []string{string(hash[:])}, nil
}
func (t HashTokenizer) Identifier() byte { return 0xB }
func (t HashTokenizer) IsSortable() bool { return false }
func (t HashTokenizer) IsLossy() bool    { return true }
