package types

import (
	"bytes"
	"encoding/binary"

	"github.com/dgraph-io/dgraph/tok"
)

const (
	// Posting list keys are prefixed with this rune if it is a mutation meant for
	// the index.
	indexRune   = ':'
	dateFormat1 = "2006-01-02"
	dateFormat2 = "2006-01-02T15:04:05"
)

// DefaultIndexKeys tokenizes data as a string and return keys for indexing.
func DefaultIndexKeys(attr string, val *String) []string {
	data := []byte((*val))
	tokenizer, err := tok.NewTokenizer(data)
	if err != nil {
		return nil
	}
	defer tokenizer.Destroy()

	tokens := make([]string, 0, 5)
	for {
		s := tokenizer.Next()
		if s == nil {
			break
		}
		tokens = append(tokens, string(s))
	}
	return tokens
}

// IntIndex indexs int type.
func IntIndex(attr string, val *Int32) ([]string, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int32(*val))
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}

// FloatIndex indexs float type.
func FloatIndex(attr string, val *Float) ([]string, error) {
	in := int32(*val)
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, in)
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}

// DateIndex indexs time type.
func DateIndex(attr string, val *Date) ([]string, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int32((*val).Time.Year()))
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}

// TimeIndex indexs time type.
func TimeIndex(attr string, val *Time) ([]string, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int32((*val).Time.Year()))
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}
