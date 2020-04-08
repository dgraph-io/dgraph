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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type ExpectedRequest struct {
	method    string
	urlSuffix string
	body      string
	// Send headers as nil to ignore comparing headers.
	// Provide nil value for a key just to ensure that the key exists in request headers.
	// Provide both key and value to ensure that key exists with given value
	headers map[string][]string
}

func getError(key, val string) error {
	return fmt.Errorf(`{ "errors": [{"message": "%s: %s"}] }`, key, val)
}

func verifyRequest(r *http.Request, expectedRequest ExpectedRequest) error {
	if r.Method != expectedRequest.method {
		return getError("Invalid HTTP method", r.Method)
	}

	if !strings.HasSuffix(r.URL.String(), expectedRequest.urlSuffix) {
		return getError("Invalid URL", r.URL.String())
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return getError("Unable to read request body", err.Error())
	}
	if string(b) != expectedRequest.body {
		return getError("Unexpected value for request body", string(b))
	}

	if expectedRequest.headers != nil {
		actualHeaderLen := len(r.Header)
		expectedHeaderLen := len(expectedRequest.headers)
		if actualHeaderLen != expectedHeaderLen {
			return getError(fmt.Sprintf("Wanted %d headers in request, got", expectedHeaderLen),
				strconv.Itoa(actualHeaderLen))
		}

		for k, v := range expectedRequest.headers {
			rv, ok := r.Header[k]
			if !ok {
				return getError("Required header not found", k)
			}

			if v == nil {
				continue
			}

			sort.Strings(rv)
			sort.Strings(v)

			if !reflect.DeepEqual(rv, v) {
				return getError(fmt.Sprintf("Unexpected value for %s header", k), fmt.Sprint(rv))
			}
		}
	}

	return nil
}

func getDefaultResponse(resKey string) []byte {
	resTemplate := `{
		"%s": [
			{
				"id": "0x3",
				"name": "Star Wars",
				"director": [
					{
						"id": "0x4",
						"name": "George Lucas"
					}
				]
			},
			{
				"id": "0x5",
				"name": "Star Trek",
				"director": [
					{
						"id": "0x6",
						"name": "J.J. Abrams"
					}
				]
			}
		]
	}`

	return []byte(fmt.Sprintf(resTemplate, resKey))
}

func getFavMoviesHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, ExpectedRequest{
		method:    http.MethodGet,
		urlSuffix: "/0x123?name=Author&num=10",
		body:      "",
		headers:   nil,
	})
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	_, _ = w.Write(getDefaultResponse("myFavoriteMovies"))
}

func postFavMoviesHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, ExpectedRequest{
		method:    http.MethodPost,
		urlSuffix: "/0x123?name=Author&num=10",
		body:      "",
		headers:   nil,
	})
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	_, _ = w.Write(getDefaultResponse("myFavoriteMoviesPost"))
}

func verifyHeadersHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, ExpectedRequest{
		method:    http.MethodGet,
		urlSuffix: "/verifyHeaders",
		body:      "",
		headers: map[string][]string{
			"X-App-Token":     {"app-token"},
			"X-User-Id":       {"123"},
			"Accept-Encoding": nil,
			"User-Agent":      nil,
		},
	})
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	_, _ = w.Write([]byte(`{"verifyHeaders":[{"id":"0x3","name":"Star Wars"}]}`))
}

func favMoviesCreateHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, ExpectedRequest{
		method:    http.MethodPost,
		urlSuffix: "/favMoviesCreate",
		body:      `{"movies":[{"director":[{"name":"Dir1"}],"name":"Mov1"},{"name":"Mov2"}]}`,
		headers:   nil,
	})
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	_, _ = w.Write([]byte(`
	{
      "createMyFavouriteMovies": [
        {
          "id": "0x1",
          "name": "Mov1",
          "director": [
            {
              "id": "0x2",
              "name": "Dir1"
            }
          ]
        },
        {
          "id": "0x3",
          "name": "Mov2"
        }
      ]
    }`))
}

func favMoviesUpdateHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, ExpectedRequest{
		method:    http.MethodPatch,
		urlSuffix: "/favMoviesUpdate/0x1",
		body:      `{"director":[{"name":"Dir1"}],"name":"Mov1"}`,
		headers:   nil,
	})
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	_, _ = w.Write([]byte(`
	{
      "updateMyFavouriteMovie": {
        "id": "0x1",
        "name": "Mov1",
        "director": [
          {
            "id": "0x2",
            "name": "Dir1"
          }
        ]
      }
    }`))
}

func favMoviesDeleteHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, ExpectedRequest{
		method:    http.MethodDelete,
		urlSuffix: "/favMoviesDelete/0x1",
		body:      "",
		headers: map[string][]string{
			"X-App-Token":     {"app-token"},
			"X-User-Id":       {"123"},
			"Accept-Encoding": nil,
			"User-Agent":      nil,
		},
	})
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	_, _ = w.Write([]byte(`
	{
      "deleteMyFavouriteMovie": {
        "id": "0x1",
        "name": "Mov1"
      }
    }`))
}

func main() {

	http.HandleFunc("/favMovies/", getFavMoviesHandler)
	http.HandleFunc("/favMoviesPost/", postFavMoviesHandler)
	http.HandleFunc("/verifyHeaders", verifyHeadersHandler)
	http.HandleFunc("/favMoviesCreate", favMoviesCreateHandler)
	http.HandleFunc("/favMoviesUpdate/", favMoviesUpdateHandler)
	http.HandleFunc("/favMoviesDelete/", favMoviesDeleteHandler)

	fmt.Println("Listening on port 8888")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
