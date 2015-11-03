/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"io/ioutil"
	"net/http"

	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("rdf")

func addRdfs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		x.SetStatus(w, x.E_INVALID_METHOD, "Invalid method")
		return
	}

	defer r.Body.Close()
	var err error
	_, err = ioutil.ReadAll(r.Body)
	if err != nil {
		x.Err(glog, err).Error("While reading request")
		x.SetStatus(w, x.E_ERROR, err.Error())
		return
	}
}

func main() {
	http.HandleFunc("/rdf", addRdfs)
}
