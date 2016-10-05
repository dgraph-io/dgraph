package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

const dgraphServer = "http://localhost:8080/query"

func decodeResponse(q string) string {
	client := new(http.Client)
	req, err := http.NewRequest("POST", dgraphServer, strings.NewReader(q))
	resp, err := client.Do(req)
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	return string(b)
}

func main() {
	q := `
    {
      me(_xid_: m.06pj8) {
        type.object.name.en
        film.director.film (first: 4)  {
          film.film.genre {
            type.object.name.en
          }
        }
      }
    }`

	expectedRes := `{"me":[{"film.director.film":[{"film.film.genre":[{"type.object.name.en":"Costume Adventure"},{"type.object.name.en":"Adventure Film"},{"type.object.name.en":"Action/Adventure"},{"type.object.name.en":"Action Film"}]},{"film.film.genre":[{"type.object.name.en":"Suspense"},{"type.object.name.en":"Drama"},{"type.object.name.en":"Adventure"},{"type.object.name.en":"Comedy"},{"type.object.name.en":"Adventure Film"},{"type.object.name.en":"Mystery film"},{"type.object.name.en":"Horror"},{"type.object.name.en":"Thriller"}]},{"film.film.genre":[{"type.object.name.en":"War film"},{"type.object.name.en":"Drama"},{"type.object.name.en":"Action Film"}]},{"film.film.genre":[{"type.object.name.en":"Drama"},{"type.object.name.en":"Adventure Film"},{"type.object.name.en":"Science Fiction"}]}],"type.object.name.en":"Steven Spielberg"}]}`
	res := decodeResponse(q)
	if res != expectedRes {
		log.Fatal("Query response is not as expected")
	}
}
