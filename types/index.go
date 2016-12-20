package types

import (
	"bytes"
	"encoding/binary"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
)

const (
	// Posting list keys are prefixed with this rune if it is a mutation meant for
	// the index.
	indexRune   = ':'
	dateFormat1 = "2006-01-02"
	dateFormat2 = "2006-01-02T15:04:05"
)

func IndexTokens(attr string, sv Val) ([]string, error) {
	switch sv.Tid {
	case GeoID:
		return IndexGeoTokens(sv.Value.(geom.T))
	case Int32ID:
		return IntIndex(attr, sv.Value.(int32))
	case FloatID:
		return FloatIndex(attr, sv.Value.(float64))
	case DateID:
		return DateIndex(attr, sv.Value.(time.Time))
	case DateTimeID:
		return TimeIndex(attr, sv.Value.(time.Time))
	case StringID:
		return DefaultIndexKeys(sv.Value.(string)), nil
	default:
		return nil, x.Errorf("Invalid type. Cannot be indexed")
	}
	return nil, nil
}

// DefaultIndexKeys tokenizes data as a string and return keys for indexing.
func DefaultIndexKeys(val string) []string {
	words := strings.Fields(val)
	tokens := make([]string, 0, 5)
	for _, it := range words {
		if it == "_nil_" {
			tokens = append(tokens, it)
		} else {
			data := []byte(it)
			tokenizer, err := tok.NewTokenizer(data)
			if err != nil {
				return nil
			}
			for {
				s := tokenizer.Next()
				if s == nil {
					break
				}
				tokens = append(tokens, string(s))
			}
			tokenizer.Destroy()
		}
	}
	return tokens
}

// IntIndex indexs int type.
func IntIndex(attr string, val int32) ([]string, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, val)
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}

// FloatIndex indexs float type.
func FloatIndex(attr string, val float64) ([]string, error) {
	in := int32(val)
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, in)
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}

// DateIndex indexs time type.
func DateIndex(attr string, val time.Time) ([]string, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int32(val.Year()))
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}

// TimeIndex indexs time type.
func TimeIndex(attr string, val time.Time) ([]string, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, int32(val.Year()))
	if err != nil {
		return nil, err
	}
	return []string{buf.String()}, nil
}
