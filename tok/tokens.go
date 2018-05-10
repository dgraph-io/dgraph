/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
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
