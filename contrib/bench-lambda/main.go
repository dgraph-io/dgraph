package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	lambda := flag.Bool("lambda", false, "Run lambda calls")
	node := flag.Bool("node", false, "Just run node JS")
	flag.Parse()

	if *node {
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			cmd := exec.Command("node", "/home/mrjn/source/dgraph-lambda/dist/index.js")
			cmd.Env = append(cmd.Env, fmt.Sprintf("PORT=%d", 8686+i))
			wg.Add(1)
			go func() {
				fmt.Printf("Running command: %+v Env: %+v\n", cmd, cmd.Env)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				if err != nil {
					log.Fatalf("Error is %v", err)
				}
			}()
		}
		wg.Wait()
	}

	client := http.Client{}
	var count int32
	start := time.Now()
	ch := make(chan struct{}, 100)
	req := func() {
		if *lambda {
			callLambda(client)
		} else {
			callDgraph(client)
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

func callLambda(client http.Client) {
	b := bytes.NewBuffer([]byte(`{"query":"query {\n  queryUser{\n    Capital\n }\n}\n"}`))
	resp, err := client.Post("http://localhost:8180/graphql", "application/json", b)
	if err != nil {
		log.Fatal(err)
	}
	// _, err = ioutil.ReadAll(resp.Body)
	bb, err := ioutil.ReadAll(resp.Body)
	if !strings.Contains(string(bb), "NAMAN") {
		log.Fatalf("Didn't get NAMAN: %s\n", bb)
	}
	// fmt.Printf(string(bb))
	resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func callDgraph(client http.Client) {
	b := bytes.NewBuffer([]byte(`{"query":"query {\n  queryUser{\n    name\n }\n}\n"}`))
	resp, err := client.Post("http://localhost:8180/graphql", "application/json", b)
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
