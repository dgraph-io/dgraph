package schema

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	store map[string]Type
)

func Parse(schema string) error {
	file, err := os.Open(schema)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var cur, curObj string
	var newObj Object
	var isScalarBlock bool
	scanner := bufio.NewScanner(file)
	for i := 0; scanner.Scan(); i++ {
		cur = strings.Trim(scanner.Text(), " \t\n")
		if cur == "" || cur[0] == '#' {
			continue
		}

		if cur[0] == '}' {
			store[curObj] = newObj
			curObj = ""
		} else if cur[0] == ')' {
			isScalarBlock = false
		} else if len(cur) > 3 && cur[:4] == "type" {
			if isScalarBlock {
				return fmt.Errorf("line %v: Cannot have type inside scalar block", i)
			}
			curObj = strings.Trim(cur[5:], " {")
			newObj = Object{
				Name:   curObj,
				Fields: make(map[string]string),
			}
		} else if len(cur) > 5 && cur[:6] == "scalar" {
			curIt := strings.Split(cur, " ")
			if len(curIt) > 1 && curIt[1] == "(" {
				isScalarBlock = true
				continue
			} else {
				temp := strings.Split(strings.Trim(cur[7:], " \t"), ":")
				name := strings.Trim(temp[0], " \t")
				typ := strings.Trim(temp[1], " \t")
				t, ok := getScalar(typ)
				if !ok {
					return fmt.Errorf("Invalid scalar type")
				}
				store[name] = t
			}

		} else {
			temp := strings.Split(cur, ":")
			if len(temp) < 2 {
				return fmt.Errorf("Invalid declaration")
			}
			name := strings.Trim(temp[0], " \t")
			typ := strings.Trim(temp[1], " \t")

			if curObj != "" {
				newObj.Fields[name] = typ
			} else if isScalarBlock {
				t, ok := getScalar(typ)
				if !ok {
					return fmt.Errorf("Invalid scalar type")
				}
				store[name] = t
			} else {
				return fmt.Errorf("Invaild schema: Line %v", i)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Error reading schema file: %v", err)
	}

	for _, v := range store {
		if obj, ok := v.(Object); ok {
			for p, q := range obj.Fields {
				typ := TypeOf(q)
				if typ != nil && !typ.IsScalar() {
					store[p] = typ
				}
			}
		}
	}

	fmt.Println(store)
	return nil
}
