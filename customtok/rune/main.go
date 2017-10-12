package main

import (
	"encoding/binary"
)

func Tokenizer() interface{} {
	return RuneTokenizer{}
}

type RuneTokenizer struct {
}

func (RuneTokenizer) Name() string     { return "rune" }
func (RuneTokenizer) Type() string     { return "string" }
func (RuneTokenizer) Identifier() byte { return 0xfd }

func (t RuneTokenizer) Tokens(value interface{}) ([]string, error) {
	var toks []string
	for _, r := range value.(string) {
		var buf [binary.MaxVarintLen32]byte
		n := binary.PutVarint(buf[:], int64(r))
		tok := string(buf[:n])
		toks = append(toks, tok)
	}
	return toks, nil
}
