package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	client := http.Client{}
	var count int32
	start := time.Now()
	ch := make(chan struct{}, 100)
	req := func() {
		callLambda(client)
		// callDgraph(client)
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
	for i := 0; i < 50000; i++ {
		ch <- struct{}{}
	}
	close(ch)
	wg.Wait()
}

func callLambda(client http.Client) {
	b := bytes.NewBuffer([]byte(`{"query":"query {\n  queryUser{\n    Capital\n }\n}\n"}`))
	resp, err := client.Post("http://localhost:8080/graphql", "application/json", b)
	if err != nil {
		log.Fatal(err)
	}
	_, err = ioutil.ReadAll(resp.Body)
	// bb, err := ioutil.ReadAll(resp.Body)
	// fmt.Printf(string(bb))
	defer resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func callDgraph(client http.Client) {
	b := bytes.NewBuffer([]byte(`{"query":"query {\n  queryUser{\n    name\n }\n}\n"}`))
	resp, err := client.Post("http://localhost:8080/graphql", "application/json", b)
	if err != nil {
		log.Fatal(err)
	}
	_, err = ioutil.ReadAll(resp.Body)
	// bb, err := ioutil.ReadAll(resp.Body)
	// fmt.Println(string(bb))
	defer resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
}
