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
	"time"

	"github.com/blevesearch/bleve/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/token/porter"
	"github.com/blevesearch/bleve/analysis/token/unicodenorm"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/registry"
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

	// Identifier returns the prefix byte for this token type.
	Identifier() byte

	// IsSortable returns if the index can be used for sorting.
	IsSortable() bool
}

const normalizerName = "nfkc_normalizer"

var (
	tokenizers map[string]Tokenizer
	defaults   map[types.TypeID]Tokenizer
	bleveCache *registry.Cache
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

	// Check for duplicate prexif bytes.
	usedIds := make(map[byte]struct{})
	for _, tok := range tokenizers {
		tokID := tok.Identifier()
		_, ok := usedIds[tokID]
		x.AssertTruef(!ok, "Same ID used by multiple tokenizers")
		usedIds[tokID] = struct{}{}
	}
	// Prepare Bleve cache
	initBleve()
}

func initBleve() {
	bleveCache = registry.NewCache()

	// Create normalizer using Normalization Form KC (NFKC) - Compatibility Decomposition, followed
	// by Canonical Composition. See: http://unicode.org/reports/tr15/#Norm_Forms
	_, err := bleveCache.DefineTokenFilter(normalizerName, map[string]interface{}{
		"type": unicodenorm.Name,
		"form": "nfkc",
	})
	if err != nil {
		panic(err)
	}

	// basic analyzer - splits on word boundaries, lowercase and normalize tokens
	_, err = bleveCache.DefineAnalyzer("term", map[string]interface{}{
		"type":          custom.Name,
		"tokenizer":     unicode.Name,
		"token_filters": []string{lowercase.Name, normalizerName},
	})
	if err != nil {
		panic(err)
	}

	// full text search analyzer - does stemming using Porter stemmer - this works only for English.
	// Per-language stemming will be added soon.
	_, err = bleveCache.DefineAnalyzer("fulltext", map[string]interface{}{
		"type":          custom.Name,
		"tokenizer":     unicode.Name,
		"token_filters": []string{lowercase.Name, normalizerName, porter.Name},
	})
	if err != nil {
		panic(err)
	}
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
	tokens, err := types.IndexGeoTokens(sv.Value.(geom.T))
	EncodeGeoTokens(tokens)
	return tokens, err
}
func (t GeoTokenizer) Identifier() byte { return 0x5 }
func (t GeoTokenizer) IsSortable() bool { return false }

type Int32Tokenizer struct{}

func (t Int32Tokenizer) Name() string       { return "int" }
func (t Int32Tokenizer) Type() types.TypeID { return types.Int32ID }
func (t Int32Tokenizer) Tokens(sv types.Val) ([]string, error) {
	return []string{encodeToken(encodeInt(sv.Value.(int32)), t.Identifier())}, nil
}
func (t Int32Tokenizer) Identifier() byte { return 0x6 }
func (t Int32Tokenizer) IsSortable() bool { return true }

type FloatTokenizer struct{}

func (t FloatTokenizer) Name() string       { return "float" }
func (t FloatTokenizer) Type() types.TypeID { return types.FloatID }
func (t FloatTokenizer) Tokens(sv types.Val) ([]string, error) {
	return []string{encodeToken(encodeInt(int32(sv.Value.(float64))), t.Identifier())}, nil
}
func (t FloatTokenizer) Identifier() byte { return 0x7 }
func (t FloatTokenizer) IsSortable() bool { return true }

type DateTokenizer struct{}

func (t DateTokenizer) Name() string       { return "date" }
func (t DateTokenizer) Type() types.TypeID { return types.DateID }
func (t DateTokenizer) Tokens(sv types.Val) ([]string, error) {
	return []string{encodeToken(encodeInt(int32(sv.Value.(time.Time).Year())), t.Identifier())}, nil
}
func (t DateTokenizer) Identifier() byte { return 0x3 }
func (t DateTokenizer) IsSortable() bool { return true }

type DateTimeTokenizer struct{}

func (t DateTimeTokenizer) Name() string       { return "datetime" }
func (t DateTimeTokenizer) Type() types.TypeID { return types.DateTimeID }
func (t DateTimeTokenizer) Tokens(sv types.Val) ([]string, error) {
	return []string{encodeToken(encodeInt(int32(sv.Value.(time.Time).Year())), t.Identifier())}, nil
}
func (t DateTimeTokenizer) Identifier() byte { return 0x4 }
func (t DateTimeTokenizer) IsSortable() bool { return true }

type TermTokenizer struct{}

func (t TermTokenizer) Name() string       { return "term" }
func (t TermTokenizer) Type() types.TypeID { return types.StringID }
func (t TermTokenizer) Tokens(sv types.Val) ([]string, error) {
	return getBleveTokens(t.Name(), t.Identifier(), sv)
}
func (t TermTokenizer) Identifier() byte { return 0x1 }
func (t TermTokenizer) IsSortable() bool { return false }

type ExactTokenizer struct{}

func (t ExactTokenizer) Name() string       { return "exact" }
func (t ExactTokenizer) Type() types.TypeID { return types.StringID }
func (t ExactTokenizer) Tokens(sv types.Val) ([]string, error) {
	term, ok := sv.Value.(string)
	if !ok {
		return nil, x.Errorf("Exact indices only supported for string types")
	}
	return []string{encodeToken(term, t.Identifier())}, nil
}
func (t ExactTokenizer) Identifier() byte { return 0x2 }
func (t ExactTokenizer) IsSortable() bool { return true }

// Full text tokenizer. Currenlty works only for English language.
type FullTextTokenizer struct{}

func (t FullTextTokenizer) Name() string       { return "fulltext" }
func (t FullTextTokenizer) Type() types.TypeID { return types.StringID }
func (t FullTextTokenizer) Tokens(sv types.Val) ([]string, error) {
	return getBleveTokens(t.Name(), t.Identifier(), sv)
}
func (t FullTextTokenizer) Identifier() byte { return 0x8 }
func (t FullTextTokenizer) IsSortable() bool { return false }

func getBleveTokens(name string, identifier byte, sv types.Val) ([]string, error) {
	analyzer, err := bleveCache.AnalyzerNamed(name)
	if err != nil {
		return nil, err
	}
	tokenStream := analyzer.Analyze([]byte(sv.Value.(string)))

	terms := make([]string, len(tokenStream))
	for i, token := range tokenStream {
		terms[i] = encodeToken(string(token.Term), identifier)
	}
	return terms, nil
}

// Full text tokenizer. Currenlty works only for English language.
type FullTextTokenizer struct{}

func (t FullTextTokenizer) Name() string       { return "fulltext" }
func (t FullTextTokenizer) Type() types.TypeID { return types.StringID }
func (t FullTextTokenizer) Tokens(sv types.Val) ([]string, error) {
	return getBleveTokens(t.Name(), t.Identifier(), sv)
}
func (t FullTextTokenizer) Identifier() byte { return 0x8 }

func getBleveTokens(name string, identifier byte, sv types.Val) ([]string, error) {
	analyzer, err := bleveCache.AnalyzerNamed(name)
	if err != nil {
		return nil, err
	}
	tokenStream := analyzer.Analyze([]byte(sv.Value.(string)))

	terms := make([]string, len(tokenStream))
	for i, token := range tokenStream {
		terms[i] = encodeToken(string(token.Term), identifier)
	}
	return terms, nil
}

func encodeInt(val int32) string {
	buf := make([]byte, 5)
	binary.BigEndian.PutUint32(buf[1:], uint32(val))
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
		tokens[i] = encodeToken(tokens[i], 0x4)
	}
}
