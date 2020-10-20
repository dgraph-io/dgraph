package fbx

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/fb"
)

func AsPosting(bs []byte) *fb.Posting {
	return fb.GetRootAsPosting(bs, 0)
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

func PostingDump(p *fb.Posting) string {
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
		facets[i] = FacetDump(f)
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
