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
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/dgraph-io/dgraph/schema"
)

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
	// TODO: Remove this code and replace with query from ui
	preds := schema.State().Predicates(1)
	kw := make([]keyword, 0, len(preds))
	for _, p := range preds {
		kw = append(kw, keyword{
			Type: "predicate",
			Name: p,
		})
	}
	kws := keywords{Keywords: kw}

	predefined := []string{"id", "_uid_", "after", "first", "offset", "count",
		"@facets", "@filter", "func", "anyofterms", "allofterms", "anyoftext", "alloftext", "leq", "geq", "or", "and",
		"orderasc", "orderdesc", "near", "within", "contains", "intersects"}

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

func replacePort(h http.Handler, indexHtml string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If path is '/', lets return the index.html that we have in memory
		// after replacing the PORT.
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			fmt.Fprint(w, indexHtml)
			return
		}

		// Else lets serve the contents (js/css) of the ui directory.
		h.ServeHTTP(w, r)
	})
}

func substitutePort() string {
	indexHtml, err := ioutil.ReadFile(*uiDir + "/index.html")
	if err == nil {
		indexHtml = bytes.Replace(indexHtml, []byte("__SERVER_PORT__"), []byte(strconv.Itoa(*port)), 1)
		return string(indexHtml)
	}
	return ""
}
