package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var lambda, verbose bool
var port int

func main() {
	flag.BoolVar(&lambda, "lambda", false, "Run lambda calls")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logs")
	flag.IntVar(&port, "port", 8080, "Verbose logs")
	flag.Parse()

	url := fmt.Sprintf("http://localhost:%d/graphql", port)

	client := http.Client{}
	var count int32
	start := time.Now()
	ch := make(chan struct{}, 100)
	req := func() {
		if lambda {
			callLambda(client, url)
		} else {
			callDgraph(client, url)
		}
		if num := atomic.AddInt32(&count, 1); num%1000 == 0 {
			elasped := time.Since(start).Round(time.Second).Seconds()
			if elasped == 0 {
				return
			}
			fmt.Printf("[Chan: %d] Done %d requests in time: %f QPS: %d\n",
				len(ch), num, elasped, num/int32(elasped))
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range ch {
				req()
			}
		}()
	}
	for i := 0; i < 200000; i++ {
		ch <- struct{}{}
	}
	close(ch)
	wg.Wait()
}

func callLambda(client http.Client, url string) {
	b := bytes.NewBuffer([]byte(`{"query":"query {\n  queryUser{\n    Capital\n }\n}\n"}`))
	req, err := http.NewRequest("POST", "http://localhost:8080/graphql", b)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	bb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Validate data.
	if !strings.Contains(string(bb), "NAMAN") {
		log.Fatalf("Didn't get NAMAN: %s\n", bb)
	}
	if verbose {
		fmt.Printf("[LAMBDA] %s\n", bb)
	}
}

func callDgraph(client http.Client, url string) {
	b := bytes.NewBuffer([]byte(`{"query":"query {\n  queryUser{\n    name\n }\n}\n"}`))
	resp, err := client.Post("http://localhost:8080/graphql", "application/json", b)
	if err != nil {
		log.Fatal(err)
	}
	bb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Validate data.
	if !strings.Contains(string(bb), "Naman") {
		log.Fatalf("Didn't get NAMAN: %s\n", bb)
	}
	if verbose {
		fmt.Printf("[DGRAPH] %s\n", bb)
	}
}
