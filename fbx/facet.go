package fbx

import (
	"bytes"
	"fmt"

	"github.com/dgraph-io/dgraph/fb"
)

func FacetTokens(f *fb.Facet) []string {
	tokens := make([]string, f.TokensLength())
	for i := 0; i < f.TokensLength(); i++ {
		tokens[i] = string(f.Tokens(i))
	}
	return tokens
}

func FacetEq(f1, f2 *fb.Facet) bool {
	return bytes.Equal(f1.Key(), f2.Key()) &&
		bytes.Equal(f1.ValueBytes(), f2.ValueBytes()) &&
		f1.ValueType() == f2.ValueType() &&
		f1.TokensLength() == f2.TokensLength() &&
		facetTokensEq(f1, f2) &&
		bytes.Equal(f1.Alias(), f2.Alias())
}

func FacetDump(f *fb.Facet) string {
	return fmt.Sprintf(
		"{key:%s value:%s value_type:%d tokens:%+v alias:%s}",
		f.Key(),
		f.ValueBytes(),
		f.ValueType(),
		FacetTokens(f),
		f.Alias(),
	)
}

func facetTokensEq(f1, f2 *fb.Facet) bool {
	n := f1.TokensLength()
	if n != f2.TokensLength() {
		return false
	}
	for i := 0; i < n; i++ {
		if !bytes.Equal(f1.Tokens(i), f2.Tokens(i)) {
			return false
		}
	}
	return true
}
