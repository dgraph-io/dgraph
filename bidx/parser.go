/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package bidx

import (
	"log"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/x"
)

type valueParser func(string) interface{}

var valueParserMap map[string]valueParser

func init() {
	valueParserMap = map[string]valueParser{
		"text":     parseText,
		"int":      parseInt,
		"float":    parseFloat,
		"bool":     parseBool,
		"datetime": parseDateTime,
	}
}

func parseText(s string) interface{} {
	return s
}

func parseInt(s string) interface{} {
	v, err := strconv.Atoi(s)
	x.Check(err)
	return v
}

func parseFloat(s string) interface{} {
	v, err := strconv.ParseFloat(s, 64) // v is float64
	x.Check(err)
	return v
}

func parseBool(s string) interface{} {
	s = strings.ToLower(strings.TrimSpace(s))
	return s != "false" && s != "0" && s != ""
}

func parseDateTime(s string) interface{} {
	log.Fatal("parseDateTime unimplemented")
	return ""
}

func getParser(s string) valueParser {
	p, found := valueParserMap[s]
	x.Assertf(found, "No parser for type %s", s)
	return p
}
