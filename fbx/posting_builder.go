package fbx

import (
	"math"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/fb"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	flatbuffers "github.com/google/flatbuffers/go"
)

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
