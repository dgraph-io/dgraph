package testing

import (
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"
)

func decodeResponse(q string) string {
	dgraphServer := "http://localhost:8080/query"
	client := new(http.Client)
	req, err := http.NewRequest("POST", dgraphServer, strings.NewReader(q))
	resp, err := client.Do(req)
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	return string(b)
}

func TestSimple(t *testing.T) {
	// Run mutation.
	m := `mutation {
        set {
            <alice-in-wonderland> <type> <novel> .
            <alice-in-wonderland> <character> <alice> .
            <alice-in-wonderland> <author> <lewis-carrol> .
            <alice-in-wonderland> <written-in> "1865" .
            <alice-in-wonderland> <name> "Alice in Wonderland" .
            <alice-in-wonderland> <sequel> <looking-glass> .
            <alice> <name> "Alice" .
            <lewis-carrol> <name> "Lewis Carroll" .
            <lewis-carrol> <born> "1832" .
            <lewis-carrol> <died> "1898" .
        }
     }`

	decodeResponse(m)

	q := `
    {
    	me(id: alice-in-wonderland) {
    		type
    		written-in
    		name
    		character {
                name
    		}
    		author {
                name
                born
                died
    		}
    	}
    }`

	expectedRes := `{"me":[{"author":[{"born":"1832","died":"1898","name":"Lewis Carroll"}],"character":[{"name":"Alice"}],"name":"Alice in Wonderland","written-in":"1865"}]}`
	res := decodeResponse(q)
	if res != expectedRes {
		log.Fatal("Query response is not as expected")
	}
}
