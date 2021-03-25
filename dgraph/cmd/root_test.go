package cmd

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
)

func TestConvertJSON(t *testing.T) {
	hier := `{
	  "mutations": "strict",
	  "badger": {
	    "compression": "zstd:1",
	    "goroutines": 5
	  },
      "raft": {
	    "idx": 2,
	    "learner": true
	  },
	  "security": {
	    "whitelist": "127.0.0.1,0.0.0.0"
	  }
	}`
	conv, err := ioutil.ReadAll(convertJSON(hier))
	if err != nil {
		t.Fatal("error reading from convertJSON")
	}
	unchanged, err := ioutil.ReadAll(convertJSON(string(conv)))
	if err != nil {
		t.Fatal("error reading from convertJSON")
	}
	if string(unchanged) != string(conv) {
		t.Fatal("convertJSON mutating already flattened string")
	}
	// need both permutations because convertJSON iterates through Go hashmaps in undefined order
	if (!strings.Contains(string(conv), "compression=zstd:1; goroutines=5;") &&
		!strings.Contains(string(conv), "goroutines=5; compression=zstd:1;")) ||
		(!strings.Contains(string(conv), "idx=2; learner=true;") &&
			!strings.Contains(string(conv), "learner=true; idx=2;")) ||
		!strings.Contains(string(conv), "whitelist=127.0.0.1,0.0.0.0") {
		fmt.Println(string(conv))
		t.Fatal("convertJSON not converting properly")
	}
}

func TestConvertYAML(t *testing.T) {
	hier := `
      mutations: strict
      badger:
        compression: zstd:1
        goroutines: 5
      raft:
        idx: 2
        learner: true
      security:
        whitelist: "127.0.0.1,0.0.0.0"`

	conv, err := ioutil.ReadAll(convertYAML(hier))
	if err != nil {
		t.Fatal("error reading from convertYAML")
	}
	unchanged, err := ioutil.ReadAll(convertYAML(string(conv)))
	if err != nil {
		t.Fatal("error reading from convertYAML")
	}
	if string(unchanged) != string(conv) {
		t.Fatal("convertYAML mutating already flattened string")
	}
	if !strings.Contains(string(conv), "compression=zstd:1; goroutines=5;") ||
		!strings.Contains(string(conv), "idx=2; learner=true;") ||
		!strings.Contains(string(conv), "whitelist=127.0.0.1,0.0.0.0") {
		t.Fatal("convertYAML not converting properly")
	}
}
