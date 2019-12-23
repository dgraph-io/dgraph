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

package alpha

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
	x.Check2(w.Write([]byte(
		"Dgraph browser is available for running separately using the dgraph-ratel binary")))
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
		"@cascade",
		"@facets",
		"@filter",
		"@if",
		"@normalize",
		"after",
		"allofterms",
		"alloftext",
		"and",
		"anyofterms",
		"anyoftext",
		"as",
		"avg",
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
		"gt",
		"index",
		"intersects",
		"le",
		"len",
		"lt",
		"mutation",
		"near",
		"not",
		"offset",
		"or",
		"orderasc",
		"orderdesc",
		"recurse",
		"regexp",
		"reverse",
		"schema",
		"set",
		"sum",
		"term",
		"tokenizer",
		"type",
		"uid",
		"within",
		"upsert",
	}

	for _, w := range predefined {
		kws.Keywords = append(kws.Keywords, keyword{
			Name: w,
		})
	}
	js, err := json.Marshal(kws)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		x.Check2(w.Write([]byte(err.Error())))
		return
	}
	x.Check2(w.Write(js))
}
