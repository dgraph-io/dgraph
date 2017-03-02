/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
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
	"strings"
	"time"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/token/porter"
	"github.com/blevesearch/bleve/analysis/token/unicodenorm"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	geom "github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// Tokenizer defines what a tokenizer must provide.
type Tokenizer interface {
	// Name is name of tokenizer. This should be unique.
	Name() string

	// Type returns typeID that we care about.
	Type() types.TypeID

	// Tokens return tokens for a given value.
	Tokens(sv types.Val) ([]string, error)
}

var (
	tokenizers map[string]Tokenizer
	defaults   map[types.TypeID]Tokenizer
)

func init() {
	RegisterTokenizer(GeoTokenizer{})
	RegisterTokenizer(Int32Tokenizer{})
	RegisterTokenizer(FloatTokenizer{})
	RegisterTokenizer(DateTokenizer{})
	RegisterTokenizer(DateTimeTokenizer{})
	RegisterTokenizer(TermTokenizer{})
	RegisterTokenizer(ExactTokenizer{})
	RegisterTokenizer(FullTextTokenizer{})
	SetDefault(types.GeoID, "geo")
	SetDefault(types.Int32ID, "int")
	SetDefault(types.FloatID, "float")
	SetDefault(types.DateID, "date")
	SetDefault(types.DateTimeID, "datetime")
	SetDefault(types.StringID, "term")
}

// GetTokenizer returns tokenizer given unique name.
func GetTokenizer(name string) Tokenizer {
	t, found := tokenizers[name]
	x.AssertTruef(found, "Tokenizer not found %s", name)
	return t
}

// Default returns the default tokenizer for a given type.
func Default(typ types.TypeID) Tokenizer {
	t, found := defaults[typ]
	x.AssertTruef(found, "No default tokenizer set for type %v", typ)
	return t
}

// SetDefault sets the default tokenizer for given typeID.
func SetDefault(typ types.TypeID, name string) {
	if defaults == nil {
		defaults = make(map[types.TypeID]Tokenizer)
	}
	t := GetTokenizer(name)
	x.AssertTruef(t.Type() == typ, "Type mismatch %v vs %v", t.Type(), typ)
	defaults[typ] = t
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
func (t GeoTokenizer) Tokens(sv types.Val) ([]string, error) {
	return types.IndexGeoTokens(sv.Value.(geom.T))
}

type Int32Tokenizer struct{}

func (t Int32Tokenizer) Name() string       { return "int" }
func (t Int32Tokenizer) Type() types.TypeID { return types.Int32ID }
func (t Int32Tokenizer) Tokens(sv types.Val) ([]string, error) {
	return encodeInt(sv.Value.(int32))
}

type FloatTokenizer struct{}

func (t FloatTokenizer) Name() string       { return "float" }
func (t FloatTokenizer) Type() types.TypeID { return types.FloatID }
func (t FloatTokenizer) Tokens(sv types.Val) ([]string, error) {
	return encodeInt(int32(sv.Value.(float64)))
}

type DateTokenizer struct{}

func (t DateTokenizer) Name() string       { return "date" }
func (t DateTokenizer) Type() types.TypeID { return types.DateID }
func (t DateTokenizer) Tokens(sv types.Val) ([]string, error) {
	return encodeInt(int32(sv.Value.(time.Time).Year()))
}

type DateTimeTokenizer struct{}

func (t DateTimeTokenizer) Name() string       { return "datetime" }
func (t DateTimeTokenizer) Type() types.TypeID { return types.DateTimeID }
func (t DateTimeTokenizer) Tokens(sv types.Val) ([]string, error) {
	return encodeInt(int32(sv.Value.(time.Time).Year()))
}

type TermTokenizer struct{}

func (t TermTokenizer) Name() string       { return "term" }
func (t TermTokenizer) Type() types.TypeID { return types.StringID }
func (t TermTokenizer) Tokens(sv types.Val) ([]string, error) {
	tokenizer := unicode.NewUnicodeTokenizer()
	toLowerFilter := lowercase.NewLowerCaseFilter()
	normalizeFilter, err := unicodenorm.NewUnicodeNormalizeFilter("nfkc")
	if err != nil {
		return nil, err
	}

	analyzer := analysis.Analyzer{
		Tokenizer: tokenizer,
		TokenFilters: []analysis.TokenFilter{
			toLowerFilter,
			normalizeFilter,
		},
	}

	tokenStream := analyzer.Analyze([]byte(sv.Value.(string)))

	return extractTerms(tokenStream), nil
}

type ExactTokenizer struct{}

func (t ExactTokenizer) Name() string       { return "exact" }
func (t ExactTokenizer) Type() types.TypeID { return types.StringID }
func (t ExactTokenizer) Tokens(sv types.Val) ([]string, error) {
	words := strings.Fields(sv.Value.(string))
	return []string{strings.Join(words, " ")}, nil
}

type FullTextTokenizer struct{}

func (t FullTextTokenizer) Name() string       { return "fulltext" }
func (t FullTextTokenizer) Type() types.TypeID { return types.StringID }
func (t FullTextTokenizer) Tokens(sv types.Val) ([]string, error) {
	tokenizer := unicode.NewUnicodeTokenizer()
	toLowerFilter := lowercase.NewLowerCaseFilter()
	normalizeFilter, err := unicodenorm.NewUnicodeNormalizeFilter("nfkc")
	if err != nil {
		return nil, err
	}
	porterFilter := porter.NewPorterStemmer()

	analyzer := analysis.Analyzer{
		Tokenizer: tokenizer,
		TokenFilters: []analysis.TokenFilter{
			toLowerFilter,
			normalizeFilter,
			porterFilter,
		},
	}

	tokenStream := analyzer.Analyze([]byte(sv.Value.(string)))

	return extractTerms(tokenStream), nil
}

func extractTerms(tokenStream analysis.TokenStream) []string {
	terms := make([]string, len(tokenStream))
	for i, token := range tokenStream {
		terms[i] = string(token.Term)
	}

	return terms
}

func encodeInt(val int32) ([]string, error) {
	buf := make([]byte, 5)
	binary.BigEndian.PutUint32(buf[1:], uint32(val))
	if val < 0 {
		buf[0] = 0
	} else {
		buf[0] = 1
	}
	return []string{string(buf)}, nil
}
