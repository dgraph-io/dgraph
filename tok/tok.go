/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package tok

import (
	"encoding/binary"
	"plugin"
	"time"

	"github.com/golang/glog"
	geom "github.com/twpayne/go-geom"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/text/collate"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

// Tokenizer identifiers are unique and can't be reused.
// The range 0x00 - 0x7f is system reserved.
// The range 0x80 - 0xff is for custom tokenizers.
// TODO: use these everywhere where we must ensure a system tokenizer.
const (
	IdentNone      = 0x0
	IdentTerm      = 0x1
	IdentExact     = 0x2
	IdentExactLang = 0x3
	IdentYear      = 0x4
	IdentMonth     = 0x41
	IdentDay       = 0x42
	IdentHour      = 0x43
	IdentGeo       = 0x5
	IdentInt       = 0x6
	IdentFloat     = 0x7
	IdentFullText  = 0x8
	IdentBool      = 0x9
	IdentTrigram   = 0xA
	IdentHash      = 0xB
	IdentCustom    = 0x80
	IdentDelimiter = 0x1f // ASCII 31 - Unit seperator
)

// Tokenizer defines what a tokenizer must provide.
type Tokenizer interface {

	// Name is name of tokenizer. This should be unique.
	Name() string

	// Type returns the string representation of the typeID that we care about.
	Type() string

	// Tokens return tokens for a given value. The tokens shouldn't be encoded
	// with the byte identifier.
	Tokens(interface{}) ([]string, error)

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

var tokenizers = make(map[string]Tokenizer)

func init() {
	registerTokenizer(GeoTokenizer{})
	registerTokenizer(IntTokenizer{})
	registerTokenizer(FloatTokenizer{})
	registerTokenizer(YearTokenizer{})
	registerTokenizer(HourTokenizer{})
	registerTokenizer(MonthTokenizer{})
	registerTokenizer(DayTokenizer{})
	registerTokenizer(ExactTokenizer{})
	registerTokenizer(BoolTokenizer{})
	registerTokenizer(TrigramTokenizer{})
	registerTokenizer(HashTokenizer{})
	registerTokenizer(TermTokenizer{})
	registerTokenizer(FullTextTokenizer{})
	setupBleve()
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

// LoadCustomTokenizer reads and loads a custom tokenizer from the given file.
func LoadCustomTokenizer(soFile string) {
	glog.Infof("Loading custom tokenizer from %q", soFile)
	pl, err := plugin.Open(soFile)
	x.Checkf(err, "could not open custom tokenizer plugin file")
	symb, err := pl.Lookup("Tokenizer")
	x.Checkf(err, `could not find symbol "Tokenizer" while loading custom tokenizer: %v`, err)

	// Let any type assertion panics occur, since they will contain a message
	// telling the user what went wrong. Otherwise it's hard to capture this
	// information to pass on to the user.
	tokenizer := symb.(func() interface{})().(PluginTokenizer)

	id := tokenizer.Identifier()
	x.AssertTruef(id >= IdentCustom,
		"custom tokenizer identifier byte must be >= 0x80, but was %#x", id)
	registerTokenizer(CustomTokenizer{PluginTokenizer: tokenizer})
}

// GetTokenizerByID tries to find a tokenizer by id in the registered list.
// Returns the tokenizer and true if found, otherwise nil and false.
func GetTokenizerByID(id byte) (Tokenizer, bool) {
	for _, t := range tokenizers {
		if id == t.Identifier() {
			return t, true
		}
	}
	return nil, false
}

// GetTokenizer returns tokenizer given unique name.
func GetTokenizer(name string) (Tokenizer, bool) {
	t, found := tokenizers[name]
	return t, found
}

// GetTokenizers returns a list of tokenizer given a list of unique names.
func GetTokenizers(names []string) ([]Tokenizer, error) {
	var tokenizers []Tokenizer
	for _, name := range names {
		t, found := GetTokenizer(name)
		if !found {
			return nil, errors.Errorf("Invalid tokenizer %s", name)
		}
		tokenizers = append(tokenizers, t)
	}
	return tokenizers, nil
}

func registerTokenizer(t Tokenizer) {
	_, ok := tokenizers[t.Name()]
	x.AssertTruef(!ok, "Duplicate tokenizer: %s", t.Name())
	_, ok = types.TypeForName(t.Type())
	x.AssertTruef(ok, "Invalid type %q for tokenizer %s", t.Type(), t.Name())
	tokenizers[t.Name()] = t
}

// GeoTokenizer generates tokens from geo data.
type GeoTokenizer struct{}

func (t GeoTokenizer) Name() string { return "geo" }
func (t GeoTokenizer) Type() string { return "geo" }
func (t GeoTokenizer) Tokens(v interface{}) ([]string, error) {
	return types.IndexGeoTokens(v.(geom.T))
}
func (t GeoTokenizer) Identifier() byte { return IdentGeo }
func (t GeoTokenizer) IsSortable() bool { return false }
func (t GeoTokenizer) IsLossy() bool    { return true }

// IntTokenizer generates tokens from integer data.
type IntTokenizer struct{}

func (t IntTokenizer) Name() string { return "int" }
func (t IntTokenizer) Type() string { return "int" }
func (t IntTokenizer) Tokens(v interface{}) ([]string, error) {
	return []string{encodeInt(v.(int64))}, nil
}
func (t IntTokenizer) Identifier() byte { return IdentInt }
func (t IntTokenizer) IsSortable() bool { return true }
func (t IntTokenizer) IsLossy() bool    { return false }

// FloatTokenizer generates tokens from floating-point data.
type FloatTokenizer struct{}

func (t FloatTokenizer) Name() string { return "float" }
func (t FloatTokenizer) Type() string { return "float" }
func (t FloatTokenizer) Tokens(v interface{}) ([]string, error) {
	return []string{encodeInt(int64(v.(float64)))}, nil
}
func (t FloatTokenizer) Identifier() byte { return IdentFloat }
func (t FloatTokenizer) IsSortable() bool { return true }
func (t FloatTokenizer) IsLossy() bool    { return true }

// YearTokenizer generates year tokens from datetime data.
type YearTokenizer struct{}

func (t YearTokenizer) Name() string { return "year" }
func (t YearTokenizer) Type() string { return "datetime" }
func (t YearTokenizer) Tokens(v interface{}) ([]string, error) {
	tval := v.(time.Time)
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.UTC().Year()))
	return []string{string(buf)}, nil
}
func (t YearTokenizer) Identifier() byte { return IdentYear }
func (t YearTokenizer) IsSortable() bool { return true }
func (t YearTokenizer) IsLossy() bool    { return true }

// MonthTokenizer generates month tokens from datetime data.
type MonthTokenizer struct{}

func (t MonthTokenizer) Name() string { return "month" }
func (t MonthTokenizer) Type() string { return "datetime" }
func (t MonthTokenizer) Tokens(v interface{}) ([]string, error) {
	tval := v.(time.Time)
	buf := make([]byte, 4)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.UTC().Year()))
	binary.BigEndian.PutUint16(buf[2:4], uint16(tval.UTC().Month()))
	return []string{string(buf)}, nil
}
func (t MonthTokenizer) Identifier() byte { return IdentMonth }
func (t MonthTokenizer) IsSortable() bool { return true }
func (t MonthTokenizer) IsLossy() bool    { return true }

// DayTokenizer generates day tokens from datetime data.
type DayTokenizer struct{}

func (t DayTokenizer) Name() string { return "day" }
func (t DayTokenizer) Type() string { return "datetime" }
func (t DayTokenizer) Tokens(v interface{}) ([]string, error) {
	tval := v.(time.Time)
	buf := make([]byte, 6)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.UTC().Year()))
	binary.BigEndian.PutUint16(buf[2:4], uint16(tval.UTC().Month()))
	binary.BigEndian.PutUint16(buf[4:6], uint16(tval.UTC().Day()))
	return []string{string(buf)}, nil
}
func (t DayTokenizer) Identifier() byte { return IdentDay }
func (t DayTokenizer) IsSortable() bool { return true }
func (t DayTokenizer) IsLossy() bool    { return true }

// HourTokenizer generates hour tokens from datetime data.
type HourTokenizer struct{}

func (t HourTokenizer) Name() string { return "hour" }
func (t HourTokenizer) Type() string { return "datetime" }
func (t HourTokenizer) Tokens(v interface{}) ([]string, error) {
	tval := v.(time.Time)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint16(buf[0:2], uint16(tval.UTC().Year()))
	binary.BigEndian.PutUint16(buf[2:4], uint16(tval.UTC().Month()))
	binary.BigEndian.PutUint16(buf[4:6], uint16(tval.UTC().Day()))
	binary.BigEndian.PutUint16(buf[6:8], uint16(tval.UTC().Hour()))
	return []string{string(buf)}, nil
}
func (t HourTokenizer) Identifier() byte { return IdentHour }
func (t HourTokenizer) IsSortable() bool { return true }
func (t HourTokenizer) IsLossy() bool    { return true }

// TermTokenizer generates term tokens from string data.
type TermTokenizer struct{}

func (t TermTokenizer) Name() string { return "term" }
func (t TermTokenizer) Type() string { return "string" }
func (t TermTokenizer) Tokens(v interface{}) ([]string, error) {
	str, ok := v.(string)
	if !ok || str == "" {
		return []string{str}, nil
	}
	tokens := termAnalyzer.Analyze([]byte(str))
	return uniqueTerms(tokens), nil
}
func (t TermTokenizer) Identifier() byte { return IdentTerm }
func (t TermTokenizer) IsSortable() bool { return false }
func (t TermTokenizer) IsLossy() bool    { return true }

// ExactTokenizer returns the exact string as a token. If collator is provided for
// any language then it also adds the language in the prefix .
type ExactTokenizer struct {
	langBase string
	cl       *collate.Collator
	buffer   *collate.Buffer
}

func (t ExactTokenizer) Name() string { return "exact" }
func (t ExactTokenizer) Type() string { return "string" }
func (t ExactTokenizer) Tokens(v interface{}) ([]string, error) {
	val, ok := v.(string)
	if !ok {
		return nil, errors.Errorf("Exact indices only supported for string types")
	}

	if t.cl == nil {
		return []string{val}, nil
	}

	encodedTerm := t.cl.KeyFromString(t.buffer, val)

	term := make([]byte, 0, len(t.langBase)+2+len(encodedTerm))
	term = append(term, []byte(t.langBase)...)
	term = append(term, IdentDelimiter)
	term = append(term, encodedTerm...)

	t.buffer.Reset()
	return []string{string(term)}, nil
}

func (t ExactTokenizer) Identifier() byte {
	if t.cl == nil {
		return IdentExact
	}
	return IdentExactLang
}
func (t ExactTokenizer) IsSortable() bool { return true }
func (t ExactTokenizer) IsLossy() bool    { return false }
func (t ExactTokenizer) Prefix() []byte {
	if t.cl == nil {
		return []byte{IdentExact}
	}
	prefix := []byte{IdentExactLang}
	prefix = append(prefix, []byte(t.langBase)...)
	prefix = append(prefix, IdentDelimiter)
	return prefix
}

// FullTextTokenizer generates full-text tokens from string data.
type FullTextTokenizer struct{ lang string }

func (t FullTextTokenizer) Name() string { return "fulltext" }
func (t FullTextTokenizer) Type() string { return "string" }
func (t FullTextTokenizer) Tokens(v interface{}) ([]string, error) {
	str, ok := v.(string)
	if !ok || str == "" {
		return []string{}, nil
	}
	lang := LangBase(t.lang)
	// pass 1 - lowercase and normalize input
	tokens := fulltextAnalyzer.Analyze([]byte(str))
	// pass 2 - filter stop words
	tokens = filterStopwords(lang, tokens)
	// pass 3 - filter stems
	tokens = filterStemmers(lang, tokens)
	// finally, return the terms.
	return uniqueTerms(tokens), nil
}
func (t FullTextTokenizer) Identifier() byte { return IdentFullText }
func (t FullTextTokenizer) IsSortable() bool { return false }
func (t FullTextTokenizer) IsLossy() bool    { return true }

// BoolTokenizer returns tokens from boolean data.
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
func (t BoolTokenizer) Identifier() byte { return IdentBool }
func (t BoolTokenizer) IsSortable() bool { return false }
func (t BoolTokenizer) IsLossy() bool    { return false }

// TrigramTokenizer returns trigram tokens from string data.
type TrigramTokenizer struct{}

func (t TrigramTokenizer) Name() string { return "trigram" }
func (t TrigramTokenizer) Type() string { return "string" }
func (t TrigramTokenizer) Tokens(v interface{}) ([]string, error) {
	value, ok := v.(string)
	if !ok {
		return nil, errors.Errorf("Trigram indices only supported for string types")
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
func (t TrigramTokenizer) Identifier() byte { return IdentTrigram }
func (t TrigramTokenizer) IsSortable() bool { return false }
func (t TrigramTokenizer) IsLossy() bool    { return true }

// HashTokenizer returns hash tokens from string data.
type HashTokenizer struct{}

func (t HashTokenizer) Name() string { return "hash" }
func (t HashTokenizer) Type() string { return "string" }
func (t HashTokenizer) Tokens(v interface{}) ([]string, error) {
	term, ok := v.(string)
	if !ok {
		return nil, errors.Errorf("Hash tokenizer only supported for string types")
	}
	// Blake2 is a hash function equivalent of SHA series, but faster. SHA is the best hash function
	// for doing checksum of content, because they have low collision ratios. See issue #2776.
	hash := blake2b.Sum256([]byte(term))
	if len(hash) == 0 {
		return nil, errors.Errorf("Hash tokenizer failed to create hash")
	}
	return []string{string(hash[:])}, nil
}
func (t HashTokenizer) Identifier() byte { return IdentHash }
func (t HashTokenizer) IsSortable() bool { return false }

// IsLossy false for the HashTokenizer. This allows us to avoid having to retrieve values
// for the returned results, and compare them against the value in the query, which is slow. There
// is very low probability of collisions with a 256-bit hash. We use that fact to speed up equality
// query operations using the hash index.
func (t HashTokenizer) IsLossy() bool { return false }

// PluginTokenizer is implemented by external plugins loaded dynamically via
// *.so files. It follows the implementation semantics of the Tokenizer
// interface.
//
// Think carefully before modifying this interface, as it would break users' plugins.
type PluginTokenizer interface {
	Name() string
	Type() string
	Tokens(interface{}) ([]string, error)
	Identifier() byte
}

// CustomTokenizer generates tokens from custom logic.
// It doesn't make sense for plugins to implement the IsSortable and IsLossy methods,
// so they're hard-coded.
type CustomTokenizer struct{ PluginTokenizer }

func (t CustomTokenizer) IsSortable() bool { return false }
func (t CustomTokenizer) IsLossy() bool    { return true }

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

// EncodeGeoTokens encodes the given list of tokens as geo tokens.
func EncodeGeoTokens(tokens []string) {
	for i := 0; i < len(tokens); i++ {
		tokens[i] = encodeToken(tokens[i], GeoTokenizer{}.Identifier())
	}
}

// EncodeRegexTokens encodes the given list of strings as regex tokens.
func EncodeRegexTokens(tokens []string) {
	for i := 0; i < len(tokens); i++ {
		tokens[i] = encodeToken(tokens[i], TrigramTokenizer{}.Identifier())
	}
}
