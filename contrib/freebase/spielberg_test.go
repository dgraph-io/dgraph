package testing

import (
	"log"
	"testing"
)

func TestSpielberg(t *testing.T) {
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
