package testing

import (
	"log"
	"testing"
)

func TestSpielberg(t *testing.T) {
	q := `
    {
      me(id: m.06pj8) {
        name@en
        director.film (first: 4)  {
            name@en
        }
      }
    }`

	res := decodeResponse(q)
	expectedRes := `{"me":[{"director.film":[{"name":"Indiana Jones and the Temple of Doom"},{"name":"Jaws"},{"name":"Saving Private Ryan"},{"name":"Close Encounters of the Third Kind"}],"name":"Steven Spielberg"}]}`
	if res != expectedRes {
		log.Fatal("Query response is not as expected")
	}
}
