package rdf

import (
	"log"
	"testing"

	_ "github.com/anacrolix/envpprof"
	"github.com/stretchr/testify/assert"
)

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)
}

type parseDocTestCase struct {
	Input  string
	Output []NQuad
	Err    bool
}

func testParseDoc(t *testing.T, tc parseDocTestCase) {
	t.Logf("parsing %q", tc.Input)
	ret, err := ParseDoc(tc.Input)
	if tc.Err {
		if err != nil {
			t.Logf("got error %s", err)
		} else {
			t.Error("expected error but got none")
		}
	} else if err != nil {
		t.Errorf("got unexpected error: %s", err)
	}
	assert.EqualValues(t, tc.Output, ret)
}

func TestParseDoc(t *testing.T) {
	testParseDoc(t, parseDocTestCase{})
	testParseDoc(t, parseDocTestCase{
		Input: "wah??",
		Err:   true,
	})
	testParseDoc(t, parseDocTestCase{
		Input: ` <universe> <answer> "42"@en .\n`,
		Output: []NQuad{
			{Subject: "universe", Predicate: "answer.en", ObjectValue: []byte("42")},
		},
		Err: true,
	})
	testParseDoc(t, parseDocTestCase{
		Input: " <universe> <answer> \"42\"@en .\n",
		Output: []NQuad{
			{Subject: "universe", Predicate: "answer.en", ObjectValue: []byte("42")},
		},
	})
}
