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
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

//  Might want to allow user to replace this.
var termTokenizer TermTokenizer
var fullTextTokenizer FullTextTokenizer

func GetTokens(funcArgs []string, indexing bool) ([]string, error) {
	return tokenize(funcArgs, termTokenizer, indexing)
}

func GetTextTokens(funcArgs []string, lang string, indexing bool) ([]string, error) {
	t, found := GetTokenizer("fulltext" + lang)
	if found {
		return tokenize(funcArgs, t, indexing)
	}
	return nil, x.Errorf("Tokenizer not found for %s", "fulltext"+lang)
}

func tokenize(funcArgs []string, tokenizer Tokenizer, indexing bool) ([]string, error) {
	if len(funcArgs) != 1 {
		return nil, x.Errorf("Function requires 1 arguments, but got %d",
			len(funcArgs))
	}
	sv := types.Val{types.StringID, funcArgs[0]}
	return tokenizer.Tokens(sv, indexing)
}
