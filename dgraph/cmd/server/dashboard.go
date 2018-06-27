/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package server

import (
	"encoding/json"
	"net/http"

	"github.com/dgraph-io/dgraph/x"
)

type keyword struct {
	// Type could be a predicate, function etc.
	Type string `json:"type"`
	Name string `json:"name"`
}

type keywords struct {
	Keywords []keyword `json:"keywords"`
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Dgraph browser is available for running separately using the dgraph-ratel binary"))
}

// Used to return a list of keywords, so that UI can show them for autocompletion.
func keywordHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	if r.Method != "GET" {
		http.Error(w, x.ErrorInvalidMethod, http.StatusBadRequest)
		return
	}

	var kws keywords
	predefined := []string{
		"@facets",
		"@filter",
		"after",
		"allofterms",
		"alloftext",
		"and",
		"anyofterms",
		"anyoftext",
		"contains",
		"count",
		"delete",
		"eq",
		"exact",
		"expand",
		"first",
		"fulltext",
		"func",
		"ge",
		"id",
		"index",
		"intersects",
		"le",
		"mutation",
		"near",
		"offset",
		"or",
		"orderasc",
		"orderdesc",
		"recurse",
		"regexp",
		"reverse",
		"schema",
		"set",
		"term",
		"tokenizer",
		"uid",
		"within",
	}

	for _, w := range predefined {
		kws.Keywords = append(kws.Keywords, keyword{
			Name: w,
		})
	}
	js, err := json.Marshal(kws)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(js)
}
