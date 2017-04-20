package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

var home = os.Getenv("GOPATH")
var fpath = flag.String("rest",
	home+"/src/github.com/dgraph-io/dgraph/food/rest.csv",
	"File path of restaurants file")

type Restaurant struct {
	Name    string
	Cuisine string
}

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	fmt.Printf("Reading from file: %s\n\n", *fpath)
	data, err := ioutil.ReadFile(*fpath)
	x.Checkf(err, "Unable to read restaurant file: %q", fpath)
	reader := csv.NewReader(bytes.NewReader(data))

	rests := make([]Restaurant, 0, 30)
	for {
		fields, err := reader.Read()
		if err == io.EOF {
			break
		}
		x.Checkf(err, "Unable to read fields")
		if len(fields) < 2 {
			continue
		}
		r := Restaurant{Name: fields[0], Cuisine: fields[1]}
		rests = append(rests, r)
	}
	x.AssertTruef(len(rests) >= 5, "Can't parse even five restaurants. Found: %d", len(rests))

	fmt.Printf("Choosing restaurant for 5 days from %d entries:\n", len(rests))

	var last Restaurant
	chosen := make(map[int]struct{})
	for len(chosen) < 5 {
		i := rand.Intn(len(rests))
		if _, found := chosen[i]; found {
			continue
		}
		this := rests[i]
		if last.Cuisine == this.Cuisine {
			continue
		}

		// Store this choice.
		chosen[i] = struct{}{}
		last = this

		fmt.Printf("Day [%d] Restaurant: %20s Cuisine: %10s\n", len(chosen), this.Name, this.Cuisine)
	}
}
