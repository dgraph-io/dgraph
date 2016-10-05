package testing

import (
	"log"
	"testing"
)

func TestLeonardo(t *testing.T) {
	q := `
    {
      me(_xid_: m.0dvmd) {
        type.object.name.en
        film.actor.film (offset: 10, first: 5){
          film.performance.film {
            type.object.name.en
            film.film.genre {
              type.object.name.en
            }
          }
        }
      }
    }`

	expectedRes := `{"me":[{"film.actor.film":[{"film.performance.film":[{"film.film.genre":[{"type.object.name.en":"Drama"},{"type.object.name.en":"Romance Film"}],"type.object.name.en":"The Great Gatsby"}]},{"film.performance.film":[{"film.film.genre":[{"type.object.name.en":"Drama"},{"type.object.name.en":"Romance Film"}],"type.object.name.en":"Romeo + Juliet"}]},{"film.performance.film":[{"film.film.genre":[{"type.object.name.en":"Drama"},{"type.object.name.en":"Adventure Film"},{"type.object.name.en":"Thriller"}],"type.object.name.en":"Blood Diamond"}]},{"film.performance.film":[{"film.film.genre":[{"type.object.name.en":"Historical drama"},{"type.object.name.en":"Drama"},{"type.object.name.en":"Romance Film"},{"type.object.name.en":"Epic film"}],"type.object.name.en":"Titanic"}]},{"film.performance.film":[{"film.film.genre":[{"type.object.name.en":"Black-and-white"},{"type.object.name.en":"Indie film"},{"type.object.name.en":"Parody"},{"type.object.name.en":"Drama"},{"type.object.name.en":"Comedy-drama"},{"type.object.name.en":"Comedy"}],"type.object.name.en":"Celebrity"}]}],"type.object.name.en":"Leonardo DiCaprio"}]}`
	res := decodeResponse(q)
	if res != expectedRes {
		log.Fatal("Query response is not as expected")
	}
}
