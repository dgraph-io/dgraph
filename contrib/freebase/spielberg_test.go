package testing

import (
	"log"
	"testing"
)

func TestSpielberg(t *testing.T) {
	q := `
    {
      me(id: m.06pj8) {
        name.en
        director.film (first: 4)  {
            name.en
        }
      }
    }`

	res := decodeResponse(q)
	expectedRes := `{"me":[{"director.film":[{"name.en":"Indiana Jones and the Temple of Doom"},{"name.en":"Jaws"},{"name.en":"Saving Private Ryan"},{"name.en":"Close Encounters of the Third Kind"}],"name.en":"Steven Spielberg"}]}`
	fmt.Pritln("res", res)
	if res != expectedRes {
		log.Fatal("Query response is not as expected")
	}
}
