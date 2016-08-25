package main

import (
	"fmt"
	"log"

	"github.com/dgraph-io/dgraph/bidx"
	"github.com/dgraph-io/dgraph/store"
)

//func createSchema() *bidx.Schema {
//	return &bidx.Schema{
//		E: []*bidx.Entry{
//			&bidx.Entry{
//				Type: "text",
//				Name: "film.film.genre",
//			},
//			&bidx.Entry{
//				Type: "datetime",
//				Name: "film.film.initial_release_date",
//			},
//		},
//	}
//}

func createSchema() bidx.Schema {
	return bidx.Schema{
		&bidx.Entry{
			Type: "text",
			Name: "film.film.genre",
		},
		&bidx.Entry{
			Type: "datetime",
			Name: "film.film.initial_release_date",
		},
	}
}

func main() {
	{
		s := createSchema()
		if err := s.Write("/tmp"); err != nil {
			log.Fatal(err)
		}
		fmt.Println("schema created")
	}

	schema, err := bidx.NewSchema("/tmp")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("schema read")
	fmt.Printf("Number of entries: %d\n", len(schema))
	if len(schema) != 2 {
		log.Fatalf("wrong number of entries: %d", len(schema))
	}

	ps := new(store.Store)
	postingDir := "/Users/jchiu/dgraph/films/p"
	if err := ps.InitReadOnly(postingDir); err != nil {
		log.Fatalf("error initializing postings store: %s", err)
	}
	defer ps.Close()
	fmt.Println("store opened")

	//	bidx.Backfill(ps, schema)
}
