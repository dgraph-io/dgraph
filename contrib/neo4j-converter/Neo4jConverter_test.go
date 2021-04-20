package main

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestParsingHeader(t *testing.T) {
	i := strings.NewReader("my request")
	buf := new(bytes.Buffer)
	err := processNeo4jCSV(i, buf)
	require.Contains(t, err.Error(), "column '_start' is absent")
	//log output
	fmt.Println(buf.String())
}

func TestSingleLineFile(t *testing.T) {
	i := strings.NewReader("\"_id\",\"_labels\",\"born\",\"name\",\"released\",\"tagline\",\"title\",\"_start\",\"_end\",\"_type\",\"roles\"\n\"188\",\":Movie\",\"\",\"\",\"1999\",\"Welcome to the Real World\",\"The Matrix\",,,,")
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	//log output
	fmt.Println(buf.String())
}
