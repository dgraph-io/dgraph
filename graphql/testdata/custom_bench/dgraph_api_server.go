/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

const (
	graphqlUrl = "http://localhost:8180/graphql"
)

var (
	httpClient = &http.Client{}
)

func main() {
	http.HandleFunc("/getType", getType)
	http.HandleFunc("/getBatchType", getBatchType)

	log.Println("Starting dgraph_api_server at localhost:9000 ...")
	if err := http.ListenAndServe("localhost:9000", nil); err != nil {
		log.Fatal(err)
	}
}

func getType(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	field := r.URL.Query().Get("field")
	typ := r.URL.Query().Get("type")

	if id == "" || field == "" || typ == "" {
		fmt.Println("id: ", id, ", field: ", field, ", type: ", typ)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	queryName := fmt.Sprintf("get%s", typ)
	query := fmt.Sprintf(`query {
	%s(id: "%s") {
		%s
	}
}`, queryName, id, field)
	resp, err := makeGqlRequest(query)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	val, ok := resp.Data[queryName].(map[string]interface{})
	if !ok {
		log.Println("Not found: ", queryName)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	b, err := json.Marshal(val[field])
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(b); err != nil {
		log.Println(err)
	}
}

func getBatchType(w http.ResponseWriter, r *http.Request) {
	field := r.URL.Query().Get("field")
	typ := r.URL.Query().Get("type")
	idBytes, err := ioutil.ReadAll(r.Body)

	if err != nil || field == "" || typ == "" {
		log.Println("err: ", err, ", field: ", field, ", type: ", typ)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(idBytes) == 0 {
		log.Println("no ids given")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var idObjects []Object
	err = json.Unmarshal(idBytes, &idObjects)
	if err != nil {
		log.Println("JSON unmarshal err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	idBytes = nil

	var idsBuilder strings.Builder
	comma := ""
	for _, idObject := range idObjects {
		idsBuilder.WriteString(comma)
		idsBuilder.WriteString(`"`)
		idsBuilder.WriteString(idObject.Id)
		idsBuilder.WriteString(`"`)
		comma = ","
	}
	idObjects = nil

	queryName := fmt.Sprintf("query%s", typ)
	query := fmt.Sprintf(`query {
	%s(filter: {id: [%s]}) {
		%s
	}
}`, queryName, idsBuilder.String(), field)
	idsBuilder.Reset()

	resp, err := makeGqlRequest(query)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	queryRes, ok := resp.Data[queryName].([]interface{})
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		log.Println("Not found: ", queryName)
		return
	}

	respList := make([]interface{}, 0)
	for _, nodeRes := range queryRes {
		nodeResMap, ok := nodeRes.(map[string]interface{})
		if !ok {
			log.Println("can't convert nodeRes to map: ", nodeRes)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		respList = append(respList, nodeResMap[field])
	}

	b, err := json.Marshal(respList)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(b); err != nil {
		log.Println(err)
	}
}

type Object struct {
	Id string `json:"id"`
}

type Response struct {
	Data   map[string]interface{}
	Errors interface{}
}

type GraphQLParams struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func makeGqlRequest(query string) (*Response, error) {
	params := GraphQLParams{
		Query: query,
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, graphqlUrl, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var gqlResp Response
	err = json.Unmarshal(b, &gqlResp)

	return &gqlResp, err
}
