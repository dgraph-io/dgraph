/*
 * Copyright 2017 DGraph Labs, Inc.
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

package main

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/x"
)

func homeHandler(h http.Handler, reg *regexp.Regexp) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If path is '/hexValue', lets return the index.html.
		if reg.MatchString(r.URL.Path) {
			http.ServeFile(w, r, uiDir+"/index.html")
			return
		}

		h.ServeHTTP(w, r)
	})
}

type keyword struct {
	// Type could be a predicate, function etc.
	Type string `json:"type"`
	Name string `json:"name"`
}

type keywords struct {
	Keywords []keyword `json:"keywords"`
}

// Used to return a list of keywords, so that UI can show them for autocompletion.
func keywordHandler(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method != "GET" {
		http.Error(w, x.ErrorInvalidMethod, http.StatusBadRequest)
		return
	}

	var kws keywords
	predefined := []string{
		"@facets",
		"@filter",
		"_uid_",
		"after",
		"allofterms",
		"alloftext",
		"and",
		"anyofterms",
		"anyoftext",
		"contains",
		"count",
		"delete",
		"exact",
		"first",
		"fulltext",
		"func",
		"geq",
		"id",
		"index",
		"intersects",
		"leq",
		"mutation",
		"near",
		"offset",
		"or",
		"orderasc",
		"orderdesc",
		"recurse",
		"regex",
		"reverse",
		"schema",
		"set",
		"term",
		"tokenizer",
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

func hasOnlySharePred(mutation *graphp.Mutation) bool {
	for _, nq := range mutation.Set {
		if nq.Predicate != "_share_" && nq.Predicate != "_share_hash_" {
			return false
		}
	}

	return true
}
