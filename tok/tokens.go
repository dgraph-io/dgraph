/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"github.com/dgraph-io/dgraph/x"
)

//  Might want to allow user to replace this.
var termTokenizer TermTokenizer
var fullTextTokenizer FullTextTokenizer

func GetTokens(funcArgs []string) ([]string, error) {
	return tokenize(funcArgs, termTokenizer)
}

func GetTextTokens(funcArgs []string, lang string) ([]string, error) {
	t, found := GetTokenizer("fulltext" + lang)
	if found {
		return tokenize(funcArgs, t)
	}
	return nil, x.Errorf("Tokenizer not found for %s", "fulltext"+lang)
}

func tokenize(funcArgs []string, tokenizer Tokenizer) ([]string, error) {
	if len(funcArgs) != 1 {
		return nil, x.Errorf("Function requires 1 arguments, but got %d",
			len(funcArgs))
	}
	return BuildTokens(funcArgs[0], tokenizer)
}
