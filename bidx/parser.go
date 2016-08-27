package bidx

import (
	"log"
	"strconv"
	"strings"
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
	if err != nil {
		log.Fatal(err)
	}
	return v
}

func parseFloat(s string) interface{} {
	v, err := strconv.ParseFloat(s, 64) // v is float64
	if err != nil {
		log.Fatal(err)
	}
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
	if !found {
		log.Fatalf("No parser for type %s", s)
	}
	return p
}
