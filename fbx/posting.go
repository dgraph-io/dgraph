package fbx

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/fb"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	flatbuffers "github.com/google/flatbuffers/go"
)

func AsPosting(bs []byte) *fb.Posting {
	return fb.GetRootAsPosting(bs, 0)
}

func DumpPosting(f *fb.Posting) {
	fmt.Println(postingString(f))
}

func PostingEq(p1, p2 *fb.Posting) bool {
	return p1.Uid() == p2.Uid() &&
		bytes.Equal(p1.ValueBytes(), p2.ValueBytes()) &&
		p1.ValueType() == p2.ValueType() &&
		bytes.Equal(p1.LangTagBytes(), p2.LangTagBytes()) &&
		bytes.Equal(p1.Label(), p2.Label()) &&
		p1.FacetsLength() == p2.FacetsLength() &&
		postingFacetsEq(p1, p2) &&
		p1.Op() == p2.Op() &&
		p1.StartTs() == p2.StartTs() &&
		p1.CommitTs() == p2.CommitTs()
}

func PostingFacets(p *fb.Posting) []*api.Facet {
	facets := make([]*api.Facet, p.FacetsLength())
	for i := 0; i < p.FacetsLength(); i++ {
		var facet *fb.Facet
		p.Facets(facet, i)
		facets[i] = &api.Facet{
			Key:     string(facet.Key()),
			Value:   facet.ValueBytes(),
			ValType: api.Facet_ValType(facet.ValueType()),
			Tokens:  FacetTokens(facet),
			Alias:   string(facet.Alias()),
		}
	}
	return facets
}

func postingFacetsEq(p1, p2 *fb.Posting) bool {
	n := p1.FacetsLength()
	if n != p2.FacetsLength() {
		return false
	}
	for i := 0; i < n; i++ {
		var f1, f2 *fb.Facet
		p1.Facets(f1, i)
		p2.Facets(f2, i)
		if !FacetEq(f1, f2) {
			return false
		}
	}
	return true
}

func postingString(p *fb.Posting) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(
		"{uid:%d value:%s value_type:%d posting_type:%s lang_tag:%s label:%s facets:[",
		p.Uid(),
		p.ValueBytes(),
		p.ValueType(),
		p.PostingType().String(),
		p.LangTagBytes(),
		p.Label(),
	))

	facets := make([]string, p.FacetsLength())
	for i := 0; i < p.FacetsLength(); i++ {
		var f *fb.Facet
		p.Facets(f, i)
		facets[i] = facetString(f)
	}

	sb.WriteString(fmt.Sprintf(
		"%s] op:%d start_ts:%d commit_ts:%d}",
		strings.Join(facets, " "),
		p.Op(),
		p.StartTs(),
		p.CommitTs(),
	))

	return sb.String()
}

type Posting struct {
	builder *flatbuffers.Builder

	uid         uint64
	value       flatbuffers.UOffsetT
	valueType   pb.PostingValType
	postingType fb.PostingType
	langTag     flatbuffers.UOffsetT
	label       flatbuffers.UOffsetT
	facets      flatbuffers.UOffsetT
	op          uint32
	startTs     uint64
	commitTs    uint64
}

func NewPosting() *Posting {
	const bufSize = 2 << 10
	return &Posting{
		builder: flatbuffers.NewBuilder(bufSize),
	}
}

func (p *Posting) SetUid(uid uint64) *Posting {
	p.uid = uid
	return p
}

func (p *Posting) SetValue(value []byte) *Posting {
	p.value = p.builder.CreateByteVector(value)
	return p
}

func (p *Posting) SetValueType(valueType pb.PostingValType) *Posting {
	p.valueType = valueType
	return p
}

func (p *Posting) SetPostingType(postingType fb.PostingType) *Posting {
	p.postingType = postingType
	return p
}

func (p *Posting) SetLangTag(langTag []byte) *Posting {
	p.langTag = p.builder.CreateByteVector(langTag)
	return p
}

func (p *Posting) SetLabel(label string) *Posting {
	p.label = p.builder.CreateString(label)
	return p
}

func (p *Posting) SetFacets(facets []*api.Facet) *Posting {
	offsets := make([]flatbuffers.UOffsetT, len(facets))

	for _, facet := range facets {
		f := &Facet{
			builder: p.builder,
		}
		offset := f.CopyFrom(facet).buildOffset()
		offsets = append(offsets, offset)
	}

	fb.PostingStartFacetsVector(p.builder, len(offsets))
	for i := len(offsets) - 1; i >= 0; i-- {
		p.builder.PrependUOffsetT(offsets[i])
	}
	p.facets = p.builder.EndVector(len(offsets))

	return p
}

func (p *Posting) SetOp(op uint32) *Posting {
	p.op = op
	return p
}

func (p *Posting) SetStartTs(startTs uint64) *Posting {
	p.startTs = startTs
	return p
}

func (p *Posting) SetCommitTs(commitTs uint64) *Posting {
	p.commitTs = commitTs
	return p
}

func (p *Posting) Build() *fb.Posting {
	fb.PostingStart(p.builder)
	fb.PostingAddUid(p.builder, math.MaxUint64)
	fb.PostingAddValue(p.builder, p.value)
	fb.PostingAddValueType(p.builder, int32(p.valueType))
	fb.PostingAddPostingType(p.builder, p.postingType)
	fb.PostingAddLangTag(p.builder, p.langTag)
	fb.PostingAddLabel(p.builder, p.label)
	fb.PostingAddFacets(p.builder, p.facets)
	fb.PostingAddOp(p.builder, math.MaxUint32)
	fb.PostingAddStartTs(p.builder, math.MaxUint32)
	fb.PostingAddCommitTs(p.builder, math.MaxUint32)
	offset := fb.PostingEnd(p.builder)

	p.builder.Finish(offset)
	buf := p.builder.FinishedBytes()
	posting := fb.GetRootAsPosting(buf, 0)

	// Flatbuffers do not store fields if they are set to their default values,
	// which means they cannot be mutated later. To prevent this, we set dummy
	// values in the builder and mutate them later.
	x.AssertTrue(posting.MutateUid(p.uid))
	x.AssertTrue(posting.MutateOp(p.op))
	x.AssertTrue(posting.MutateStartTs(p.startTs))
	x.AssertTrue(posting.MutateCommitTs(p.startTs))

	return posting
}
