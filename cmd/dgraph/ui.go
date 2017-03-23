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
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
)

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
