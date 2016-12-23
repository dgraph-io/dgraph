package testing

import (
	"fmt"
	"log"
	"testing"
)

func TestSpielberg(t *testing.T) {
	q := `
    {
      me(_xid_: m.06pj8) {
        type.object.name.en
        film.director.film (first: 4)  {
            type.object.name.en
        }
      }
    }`

	res := decodeResponse(q)
	expectedRes := `{"me":[{"film.director.film":[{"type.object.name.en":"Indiana Jones and the Temple of Doom"},{"type.object.name.en":"Jaws"},{"type.object.name.en":"Saving Private Ryan"},{"type.object.name.en":"Close Encounters of the Third Kind"}],"type.object.name.en":"Steven Spielberg"}]}`
	fmt.Println()
	fmt.Println(res)
	fmt.Println()
	if res != expectedRes {
		log.Fatal("Query response is not as expected")
	}
}
