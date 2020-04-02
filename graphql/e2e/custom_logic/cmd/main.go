package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"sort"
	"strings"
)

func handleCustomRequest(r *http.Request, expectedMethod, resKey string) ([]byte, error) {
	if r.Method != expectedMethod {
		return nil, fmt.Errorf(`{ "errors": [{"message": "Invalid HTTP method: %s"}] }`,
			r.Method)
	}

	if !strings.HasSuffix(r.URL.String(), "/0x123?name=Author&num=10") {
		return nil, fmt.Errorf(`{ "errors": [{"message": "Invalid URL: %s"}] }`, r.URL.String())
	}

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

	return []byte(fmt.Sprintf(resTemplate, resKey)), nil
}

func getFavMoviesHandler(w http.ResponseWriter, r *http.Request) {
	b, err := handleCustomRequest(r, http.MethodGet, "myFavoriteMovies")
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(b)
}

func postFavMoviesHandler(w http.ResponseWriter, r *http.Request) {
	b, err := handleCustomRequest(r, http.MethodPost, "myFavoriteMoviesPost")
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(b)
}

func verifyHeadersHandler(w http.ResponseWriter, r *http.Request) {
	headers := r.Header
	expectedKeys := []string{"Accept-Encoding", "User-Agent", "X-App-Token", "X-User-Id"}

	actualKeys := make([]string, 0, len(headers))
	for k := range headers {
		actualKeys = append(actualKeys, k)
	}
	sort.Strings(expectedKeys)
	sort.Strings(actualKeys)
	if !reflect.DeepEqual(expectedKeys, actualKeys) {
		fmt.Fprint(w, `{"errors": [ { "message": "Expected headers not same as actual." } ]}`)
		return
	}

	appToken := r.Header.Get("X-App-Token")
	if appToken != "app-token" {
		fmt.Fprintf(w, `{"errors": [ { "message": "Unexpected value for X-App-Token header: %s." } ]}`, appToken)
		return
	}
	userId := r.Header.Get("X-User-Id")
	if userId != "123" {
		fmt.Fprintf(w, `{"errors": [ { "message": "Unexpected value for X-User-Id header: %s." } ]}`, userId)
		return
	}
	fmt.Fprintf(w, `{"verifyHeaders":[{"id":"0x3","name":"Star Wars"}]}`)
}

type input struct {
	ID string `json:"uid"`
}

func getInput(r *http.Request) ([]input, error) {
	inputBody := make([]input, 0, 10)
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println("while reading body: ", err)
		return nil, err
	}
	if err := json.Unmarshal(b, &inputBody); err != nil {
		fmt.Println("while doing JSON unmarshal: ", err)
		return nil, err
	}
	return inputBody, nil
}

func userNamesHandler(w http.ResponseWriter, r *http.Request) {
	inputBody, err := getInput(r)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	// append uname to the id and return it.
	res := make([]interface{}, 0, len(inputBody))
	for i := 0; i < len(inputBody); i++ {
		res = append(res, "uname-"+inputBody[i].ID)
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	fmt.Fprintf(w, string(b))
}

func carsHandler(w http.ResponseWriter, r *http.Request) {
	inputBody, err := getInput(r)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	fmt.Println(inputBody)
	if len(inputBody) != 3 {
		fmt.Printf("Expected input to have length 3. Got: %+v", len(inputBody))
		return
	}

	res := []interface{}{
		[]map[string]interface{}{{
			"name": "BMW",
		}},
		[]map[string]interface{}{{
			"name": "Merc",
		}},
		[]map[string]interface{}{{
			"name": "Honda",
		}},
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	fmt.Println(string(b))
	fmt.Fprintf(w, string(b))
}

func main() {

	http.HandleFunc("/favMovies/", getFavMoviesHandler)
	http.HandleFunc("/favMoviesPost/", postFavMoviesHandler)
	http.HandleFunc("/verifyHeaders", verifyHeadersHandler)
	http.HandleFunc("/userNames", userNamesHandler)
	http.HandleFunc("/cars", carsHandler)

	fmt.Println("Listening on port 8888")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
