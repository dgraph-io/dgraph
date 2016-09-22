package schema

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	schemaFile = flag.String("schema", "", "Path to schema file")
	store      map[string]Object
)

type Object struct {
	fields map[string]string //field to type relationship
}

func Parse(schema string) error {
	store = make(map[string]Object)

	file, err := os.Open(schema)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var cur, curObj string
	var newObj Object
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		cur = strings.Trim(scanner.Text(), " \t\n")

		if cur == "" || cur[0] == '#' {
			continue
		}

		if cur[0] == '}' {
			store[curObj] = newObj
		} else if cur[:4] == "type" {
			curObj = strings.Trim(cur[5:], " {")
			newObj = Object{
				fields: make(map[string]string),
			}
		} else {
			temp := strings.Split(cur, ":")
			name := strings.Trim(temp[0], " \t")
			typ := strings.Trim(temp[1], " \t")
			newObj.fields[name] = typ
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Error reading schema file:", err)
	}
	fmt.Println(store)
	return nil
}
