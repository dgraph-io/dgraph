package fbx

import (
	"bytes"
	"fmt"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/fb"
	flatbuffers "github.com/google/flatbuffers/go"
)

func FacetEq(f1, f2 *fb.Facet) bool {
	return bytes.Equal(f1.Key(), f2.Key()) &&
		bytes.Equal(f1.ValueBytes(), f2.ValueBytes()) &&
		f1.ValueType() == f2.ValueType() &&
		f1.TokensLength() == f2.TokensLength() &&
		facetTokensEq(f1, f2) &&
		bytes.Equal(f1.Alias(), f2.Alias())
}

func FacetTokens(f *fb.Facet) []string {
	tokens := make([]string, f.TokensLength())
	for i := 0; i < f.TokensLength(); i++ {
		tokens[i] = string(f.Tokens(i))
	}
	return tokens
}

func facetString(f *fb.Facet) string {
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

type Facet struct {
	builder *flatbuffers.Builder

	key       flatbuffers.UOffsetT
	value     flatbuffers.UOffsetT
	valueType int32
	tokens    flatbuffers.UOffsetT
	alias     flatbuffers.UOffsetT
}

func NewFacet() *Facet {
	return &Facet{
		builder: flatbuffers.NewBuilder(bufSize),
	}
}

func (f *Facet) CopyFrom(facet *api.Facet) *Facet {
	return f.
		SetKey(facet.Key).
		SetValue(facet.Value).
		SetValueType(facet.ValType).
		SetTokens(facet.Tokens).
		SetAlias(facet.Alias)
}

func (f *Facet) SetKey(key string) *Facet {
	f.key = f.builder.CreateString(key)
	return f
}

func (f *Facet) SetValue(value []byte) *Facet {
	f.value = f.builder.CreateByteVector(value)
	return f
}

func (f *Facet) SetValueType(valueType api.Facet_ValType) *Facet {
	f.valueType = int32(valueType)
	return f
}

func (f *Facet) SetTokens(tokens []string) *Facet {
	offsets := make([]flatbuffers.UOffsetT, len(tokens))
	for i, token := range tokens {
		offsets[i] = f.builder.CreateString(token)
	}

	fb.FacetStartTokensVector(f.builder, len(offsets))
	for i := len(offsets) - 1; i >= 0; i-- {
		f.builder.PrependUOffsetT(offsets[i])
	}
	f.tokens = f.builder.EndVector(len(offsets))

	return f
}

func (f *Facet) SetAlias(alias string) *Facet {
	f.alias = f.builder.CreateString(alias)
	return f
}

func (f *Facet) Build() *fb.Facet {
	facet := f.buildOffset()
	f.builder.Finish(facet)
	buf := f.builder.FinishedBytes()
	return fb.GetRootAsFacet(buf, 0)
}

func (f *Facet) buildOffset() flatbuffers.UOffsetT {
	fb.FacetStart(f.builder)
	fb.FacetAddKey(f.builder, f.key)
	fb.FacetAddValue(f.builder, f.value)
	fb.FacetAddValueType(f.builder, f.valueType)
	fb.FacetAddTokens(f.builder, f.tokens)
	fb.FacetAddAlias(f.builder, f.alias)
	return fb.FacetEnd(f.builder)
}
