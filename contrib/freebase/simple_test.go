package testing

import (
	"log"
	"testing"
)

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
            <alice> <name> "Алисия"@ru .
            <alice> <name> "Adélaïde"@fr .
            <lewis-carrol> <name> "Lewis Carroll" .
            <lewis-carrol> <born> "1832" .
            <lewis-carrol> <died> "1898" .
        }
     }`

	decodeResponse(m)

	q := `
    {
    	me(_xid_: alice-in-wonderland) {
    		type
    		written-in
    		name
    		character {
                name
    			name.fr
    			name.ru
    		}
    		author {
                name
                born
                died
    		}
    	}
    }`

	expectedRes := `{"me":[{"author":[{"born":"1832","died":"1898","name":"Lewis Carroll"}],"character":[{"name":"Alice","name.fr":"Adélaïde","name.ru":"Алисия"}],"name":"Alice in Wonderland","type":[{}],"written-in":"1865"}]}`
	res := decodeResponse(q)
	if res != expectedRes {
		log.Fatal("Query response is not as expected")
	}
}
