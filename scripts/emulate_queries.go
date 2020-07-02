package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

const BASE_URL = "http://localhost:8080"

func post(contentType string, suffixURL string, data []byte) string {
	req, err := http.NewRequest("POST", BASE_URL+suffixURL, bytes.NewBuffer(data))
	//req.Header.Set("X-Custom-Header", "myvalue")
	req.Header.Set("Content-Type", contentType)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

	return string(body)
}

func read_query(res chan string) {
	data, err := ioutil.ReadFile("read_nodes.graphql")
	if err != nil {
		log.Println("Error")
	}

	body := post("application/graphql+-", "/query", data)
	res <- body
}

func mutate_schema_query(res chan string) {
	data, err := ioutil.ReadFile("mutate_schema.graphql")
	if err != nil {
		log.Println("Error")
	}
	print(string(data))
	body := post("application/json", "/alter", data)
	res <- body
}

func write_query(res chan string) {
	data, err := ioutil.ReadFile("mutate_data.graphql")
	if err != nil {
		log.Println("Error")
	}

	body := post("application/rdf", "/mutate", data)
	res <- body
}

func export_query(res chan string) {
	data, err := ioutil.ReadFile("export.graphql")
	if err != nil {
		log.Println("Error")
	}

	body := post("application/graphql", "/admin", data)
	res <- body
}

func main() {
	res1 := make(chan string)
	res2 := make(chan string)
	res3 := make(chan string)
	go read_query(res1)
	go mutate_schema_query(res2)
	go write_query(res3)
	//go export_query(res2)
	<-res1
	<-res2
	<-res3
}
