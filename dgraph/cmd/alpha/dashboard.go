/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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
	if r.URL.Path != "/" {
		http.Error(w, "404 page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	dgraph := `<body>
	<BR><h2>Dgraph is up and running...🎉🎉<BR><BR></h2>
	<p>Now you can access your cluster via Ratel Dashboard.<BR>
	Remember, the PORT to use there to connect to Alpha is 8080 + offset if the case.<BR><BR>
	if you are running GraphQL you can use any GraphQL Client from the open source community.</p>
	<p><BR><em>Thank you for using Dgraph!</em></p>
	<p><BR>Important Links: <BR>
	<a href="https://dgraph.io/docs" target="_blank">https://dgraph.io/docs</a> <BR>
	<a href="https://discuss.dgraph.io/" target="_blank">https://discuss.dgraph.io/</a> <BR>
	<a href="https://cloud.dgraph.io/" target="_blank">https://cloud.dgraph.io/</a></p> <BR>
	<p><BR><BR><BR> <div>Copyright 2016-2022 Dgraph Labs, Inc. and Contributors.</div>
	</html>`

	head := `<!DOCTYPE html>
	<html>
	<head>
	<title> Dgraph Alpha 🚀</title>
	<style>
	body{
		width:35em;
		margin:0 auto;
		font-family:Tahoma,Verdana,Arial,sans-serif;
	     }
	h2 {
		font-size: 150%;
	   }
	p {
		font-size: 107%;
	  }
	a:link {
		color: #3391ff;
	  }
	a:hover {
		color: hotpink;
	  }
	 div {
		position: absolute; 
		left: 0; 
		right: 0; 
		margin-left: auto; 
		margin-right: auto; 
		width: 469px;
	  }
	</style>
	</head> `

	x.Check2(w.Write([]byte(head + dgraph)))
}

// Used to return a list of keywords, so that UI can show them for autocompletion.
func keywordHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}

	if r.Method != http.MethodGet {
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
		"ceil",
		"cond",
		"contains",
		"count",
		"delete",
		"eq",
		"exact",
		"exp",
		"expand",
		"first",
		"floor",
		"fulltext",
		"func",
		"ge",
		"gt",
		"index",
		"intersects",
		"le",
		"len",
		"ln",
		"logbase",
		"lt",
		"math",
		"max",
		"min",
		"mutation",
		"near",
		"not",
		"offset",
		"or",
		"orderasc",
		"orderdesc",
		"pow",
		"recurse",
		"regexp",
		"reverse",
		"schema",
		"since",
		"set",
		"sqrt",
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
