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

	"github.com/dgraph-io/dgraph/x"
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
		x.Check2(w.Write([]byte(err.Error())))
		return
	}
	x.Check2(w.Write(b))
}

func postFavMoviesHandler(w http.ResponseWriter, r *http.Request) {
	b, err := handleCustomRequest(r, http.MethodPost, "myFavoriteMoviesPost")
	if err != nil {
		x.Check2(w.Write([]byte(err.Error())))
		return
	}
	x.Check2(w.Write(b))
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

func (i input) Name() string {
	return "uname-" + i.ID
}

func getInput(r *http.Request, v interface{}) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println("while reading body: ", err)
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		fmt.Println("while doing JSON unmarshal: ", err)
		return err
	}
	return nil
}

func userNamesHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []input
	err := getInput(r, &inputBody)
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

type tinput struct {
	ID string `json:"tid"`
}

func (i tinput) Name() string {
	return "tname-" + i.ID
}

func teacherNamesHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []tinput
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	// append tname to the id and return it.
	res := make([]interface{}, 0, len(inputBody))
	for i := 0; i < len(inputBody); i++ {
		res = append(res, "tname-"+inputBody[i].ID)
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	fmt.Fprintf(w, string(b))
}

type sinput struct {
	ID string `json:"sid"`
}

func (i sinput) Name() string {
	return "sname-" + i.ID
}

func schoolNamesHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []sinput
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	// append sname to the id and return it.
	res := make([]interface{}, 0, len(inputBody))
	for i := 0; i < len(inputBody); i++ {
		res = append(res, "sname-"+inputBody[i].ID)
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	fmt.Fprintf(w, string(b))
}

func carsHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []input
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	res := []interface{}{}
	for i := 0; i < len(inputBody); i++ {
		res = append(res, map[string]interface{}{
			"name": "car-" + inputBody[i].ID,
		})
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	fmt.Fprintf(w, string(b))
}

func classesHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []sinput
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	res := []interface{}{}
	for i := 0; i < len(inputBody); i++ {
		res = append(res, []map[string]interface{}{{
			"name": "class-" + inputBody[i].ID,
		}})
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	fmt.Fprintf(w, string(b))
}

type entity interface {
	Name() string
}

func nameHandler(w http.ResponseWriter, r *http.Request, input entity) {
	err := getInput(r, input)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	n := fmt.Sprintf(`"%s"`, input.Name())
	fmt.Fprintf(w, n)
}

func userNameHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody input
	nameHandler(w, r, &inputBody)
}

func carHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody input
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	res := map[string]interface{}{
		"name": "car-" + inputBody.ID,
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	fmt.Fprintf(w, string(b))
}

func classHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody sinput
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	res := make(map[string]interface{})
	res["name"] = "class-" + inputBody.ID

	b, err := json.Marshal([]interface{}{res})
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	fmt.Fprintf(w, string(b))
}

func teacherNameHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody tinput
	nameHandler(w, r, &inputBody)
}

func schoolNameHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody sinput
	nameHandler(w, r, &inputBody)
}

func main() {

	http.HandleFunc("/favMovies/", getFavMoviesHandler)
	http.HandleFunc("/favMoviesPost/", postFavMoviesHandler)
	http.HandleFunc("/verifyHeaders", verifyHeadersHandler)

	// for testing batch mode
	http.HandleFunc("/userNames", userNamesHandler)
	http.HandleFunc("/cars", carsHandler)
	http.HandleFunc("/classes", classesHandler)
	http.HandleFunc("/teacherNames", teacherNamesHandler)
	http.HandleFunc("/schoolNames", schoolNamesHandler)

	// for testing single mode
	http.HandleFunc("/userName", userNameHandler)
	http.HandleFunc("/car", carHandler)
	http.HandleFunc("/class", classHandler)
	http.HandleFunc("/teacherName", teacherNameHandler)
	http.HandleFunc("/schoolName", schoolNameHandler)

	fmt.Println("Listening on port 8888")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
