package types

import (
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

func IndexTokens(sv Val) ([]string, error) {
	switch sv.Tid {
	case GeoID:
		return IndexGeoTokens(sv.Value.(geom.T))
	case Int32ID:
		return IntIndex(sv.Value.(int32))
	case FloatID:
		return FloatIndex(sv.Value.(float64))
	case DateID:
		return DateIndex(sv.Value.(time.Time))
	case DateTimeID:
		return TimeIndex(sv.Value.(time.Time))
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
			continue
		}

		tokenizer, err := tok.NewTokenizer([]byte(it))
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
	return tokens
}

func encodeInt(val int32) ([]string, error) {
	buf := make([]byte, 5)
	binary.BigEndian.PutUint32(buf[1:], uint32(val))
	if val < 0 {
		buf[0] = 0
	} else {
		buf[0] = 1
	}
	return []string{string(buf)}, nil
}

// IntIndex indexs int type.
func IntIndex(val int32) ([]string, error) {
	return encodeInt(val)
}

// FloatIndex indexs float type.
func FloatIndex(val float64) ([]string, error) {
	in := int32(val)
	return encodeInt(in)
}

// DateIndex indexs time type.
func DateIndex(val time.Time) ([]string, error) {
	in := int32(val.Year())
	return encodeInt(in)
}

// TimeIndex indexs time type.
func TimeIndex(val time.Time) ([]string, error) {
	in := int32(val.Year())
	return encodeInt(in)
}
