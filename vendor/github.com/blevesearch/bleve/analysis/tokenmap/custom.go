//  Copyright (c) 2014 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// package token_map implements a generic TokenMap, often used in conjunction
// with filters to remove or process specific tokens.
//
// Its constructor takes the following arguments:
//
// "filename" (string): the path of a file listing the tokens. Each line may
// contain one or more whitespace separated tokens, followed by an optional
// comment starting with a "#" or "|" character.
//
// "tokens" ([]interface{}): if "filename" is not specified, tokens can be
// passed directly as a sequence of strings wrapped in a []interface{}.
package tokenmap

import (
	"fmt"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const Name = "custom"

func GenericTokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()

	// first: try to load by filename
	filename, ok := config["filename"].(string)
	if ok {
		err := rv.LoadFile(filename)
		return rv, err
	}
	// next: look for an inline word list
	tokens, ok := config["tokens"].([]interface{})
	if ok {
		for _, token := range tokens {
			tokenStr, ok := token.(string)
			if ok {
				rv.AddToken(tokenStr)
			}
		}
		return rv, nil
	}
	return nil, fmt.Errorf("must specify filename or list of tokens for token map")
}

func init() {
	registry.RegisterTokenMap(Name, GenericTokenMapConstructor)
}
