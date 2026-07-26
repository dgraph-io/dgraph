/*
 * Golden test for the conflict keys that actually reach Zero.
 *
 * Every other conflict-key test in this package compares budget=0 against
 * budget=N and reads txn.conflicts DIRECTLY. That catches a divergence between
 * the serial and parallel paths, but it cannot catch a UNIFORM regression —
 * one that changes both paths the same way (dropping the "skip zero" guard,
 * mis-deriving a key, losing a call site). It also asserts nothing about
 * FillContext, which is the function that actually converts the set into the
 * api.TxnContext.Keys sent to Zero's conflict detector.
 *
 * This test pins that output against a golden captured before the conflict-key
 * batching change, and asserts the keys do not depend on startTs (which is what
 * makes a hardcoded golden legitimate).
 */

package posting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

// goldenFillContextKeys is the exact ctx.Keys emitted by fillContextFixture at
// commit 612ae8cd5 (before conflict-key batching). Sorted and deduped by
// x.Unique inside FillContext, so it is stable.
var goldenFillContextKeys = []string{
	"10cqjlaf4j7hn", "11lxsz9zty2i4", "11rszn4j2ifrp", "15a7susihkrs5",
	"15hr881x3365h", "15ssyzlmd8o6", "160144qa0z7c9", "16g45dj9u9wtl",
	"16gddhvir9r5o", "17wz11s2rsjb0", "18r29da831jyv", "19v83k15qewy",
	"19v83k15qf35", "19v83k15qf3g", "19v83k15qf3r", "1bn2uz26zl65",
	"1csiiy24nvqav", "1csiiy24nvqb6", "1csiiy24nvqbh", "1cths9gcm15ya",
	"1cths9gcm15yj", "1cths9gcm15yo", "1cths9gcm15yx", "1cths9gcm15z2",
	"1cths9gcm15zx", "1cuw68vo5k0sx", "1d6h1mmm85zkh", "1dejkfi3t87uo",
	"1dejkfi3t87uz", "1dejkfi3t87va", "1djh0on4tr7e7", "1djh0on4tr7im",
	"1djh0on4tr7is", "1djh0on4tr7ix", "1exa3ehusmxld", "1ft3jzj1tai8s",
	"1h0g0919r4ocj", "1h0g0919r4ocs", "1h0g0919r4od5", "1h0g0919r4of6",
	"1j83u2a12v1ed", "1jdh8bcmnzjp9", "1jqs81nsjjhcd", "1mfllyn8p9mqy",
	"1ni683zm14p37", "1nzb90h02v1ji", "1o1h924e2w8mw", "1o8pxqowvsz38",
	"1ov8p3us03icd", "1q0yfx1ywiji", "1qihbzhyr587k", "1sjzq7432wmoj",
	"1u3j524i9s5ky", "1vfnls53w4667", "1whetkryqjuqv", "1whetkryqjuro",
	"1whetkryqjus1", "1whetkryqjusa", "1wxxpl268f9ai", "1xzpryyns5gd1",
	"20g6onqogazec", "21m5fh88fejnf", "25j2xbwglqjei", "26sqlzuotoul3",
	"278p0khy2r4bq", "27kronoo44soq", "27p1kzees9ayu", "281qho9c7jv02",
	"28ht75d2lsrcz", "2b3pcywmvntws", "2b8pbxzp9xdg5", "2c6y486xygbnj",
	"2cdvaxxkbeviz", "2d9ke6yxfknpj", "2ewhr6a3zocql", "2g6bgltmlq3f",
	"2g92qcof8ed3k", "2ggs3cvcwaiz7", "2gnlxpa7apx06", "2hboxqqdmpyoa",
	"2hptl0ky70li1", "2j0x0rx05xwwn", "2j7nzcgt3tg2p", "2jbbcctsnf9ro",
	"2labkkz61gs9q", "2m96ie69oxeej", "2mynehoop9r44", "2mzimus36p2hz",
	"2nwk2prfmoh9e", "2nwk2prfmohad", "2nwk2prfmoham", "2nwk2prfmohao",
	"2nwk2prfmohav", "2nwk2prfmohax", "2pug92p0njx6w", "2pug92p0njx77",
	"2pug92p0njx7i", "2qeyy1qrvq1ld", "2rauvzl3oxawo", "2wbm5aydy5tgg",
	"2z4k5bs14nthf", "304we5tysszao", "30cmwr1gt1wlh", "314vcu3mju0jp",
	"31iqzow4zlj45", "32ds5k706j92m", "32zfm4ay6zcaj", "33cfumtxb0glw",
	"33drou0jikak3", "33jj1jcfb0z98", "33jj1jcfb0z9d", "33jj1jcfb0z9y",
	"33jj1jcfb0za3", "34cn0esjmfu3o", "35jmgbjjkh8m6", "36u6n2bsbgngj",
	"383okwwymw1by", "38a2ly4gdf2vk", "3a77iqqzkqg5p", "3chush5nnfw3j",
	"3dv5y81l44ztv", "3dv5y81l44ztw", "3dv5y81l44zu5", "3dv5y81l44zu9",
	"3dv5y81l44zui", "3dv5y81l44zuw", "3ekw731h56sjb", "3fyvp2gb6k3eu",
	"3fyvp2gb6k3f1", "3fyvp2gb6k3f4", "3fyvp2gb6k3fj", "3fyvp2gb6k3hv",
	"3fyvp2gb6k3hw", "3h8iwbbbmy9gz", "3i03t92jon73g", "3iwmbwzy03xbw",
	"3jcin7c3g1zpf", "3jcin7c3g1zvr", "3jcin7c3g1zvw", "3jcin7c3g1zw5",
	"3jcin7c3g1zwa", "3kvjt2iy5h2hd", "3lv7emyt3j9x5", "3mr4yt3pdct4r",
	"3n1300qedgip9", "3p00hgoufmvxg", "3ri95zn5gkbc6", "3t8ugshh500c",
	"3tofm6wdxi5iv", "3vzymxuljypwu", "4p47rm1wmn4u", "5zvwwexrvcxu",
	"6lhiubvn7tg", "7svgt3i2uiw2", "8nyqq3jqcx8m", "8nyqq3jqcx95",
	"8nyqq3jqcxbn", "90afjmkvqjh", "9tw59jomuq26", "adauhy6jj6sg",
	"at8qvmgud4v9", "bgfl7a3jcbsc", "bzhvrptojpya", "bzhvrptojpys",
	"bzhvrptojpz3", "bzhvrptojq39", "es4kwi7vusy0", "gghvi9ijw2zm",
	"gghvi9ijw347", "gghvi9ijw34e", "gghvi9ijw34l", "gghvi9ijw34r",
	"gghvi9ijw34s", "h9ew5io0bo44", "ibbghd28i0h4", "ietvcnshejbg",
	"k131yhlfmlej", "khxohb579335", "lcc6rinevxt5", "muojk8zz9w0w",
	"muojk8zz9w1t", "muojk8zz9w22", "muojk8zz9w2n", "ozd4b9biaeu1",
	"pfx112nlxs4m", "pmr9d40ky1b4", "pmr9d40ky1bd", "pmr9d40ky1bq",
	"pmr9d40ky1bz", "pmr9d40ky1g2", "r1xso673xr1m", "scl1q4kj5tev",
	"sqyvtqgvi3ki", "ss117ufjqdru", "ttuep1j4ee49", "u5u8ncldw4k8",
	"umk34xrekawv", "v001pn39tpox", "vdbosf0xcryy", "vvhfvkcuf3bk",
	"w9k46kpeuwwr", "wmvftaeam6l3", "x36thxe2e6hn", "ytkefiufo8ur",
	"zoywn6yafefw",
}

// fillContextFixture applies a fixed multi-predicate batch exercising several
// distinct conflict-key derivations — scalar index, [uid] @reverse (which emits
// per-target keys), @upsert (bare fingerprint, no uid XOR), and @noconflict
// (must emit nothing) — then returns FillContext's ctx.Keys.
func fillContextFixture(t *testing.T, startTs uint64, budget int) []string {
	t.Helper()

	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes([]byte(`
		cname: string @index(exact) .
		clink: [uid] @reverse .
		cup:   string @index(exact) @upsert .
		cnc:   string @index(exact) @noconflict .
	`), 1))

	ob := x.WorkerConfig.MutationsPipelineGoroutines
	x.WorkerConfig.MutationsPipelineGoroutines = budget
	defer func() { x.WorkerConfig.MutationsPipelineGoroutines = ob }()

	cname := x.AttrInRootNamespace("cname")
	clink := x.AttrInRootNamespace("clink")
	cup := x.AttrInRootNamespace("cup")
	cnc := x.AttrInRootNamespace("cnc")

	var edges []*pb.DirectedEdge
	for i := 0; i < 40; i++ {
		ent := uint64(700_000 + i)
		edges = append(edges,
			strEdge(cname, ent, "n"+string(rune('a'+i%7))),
			strEdge(cup, ent, "u"+string(rune('a'+i%5))),
			strEdge(cnc, ent, "c"+string(rune('a'+i%3))),
			uidEdge(clink, ent, uint64(800_000+i%11)),
		)
	}

	txn := NewTxn(startTs)
	mp := NewMutationPipeline(txn)
	require.NoError(t, mp.Process(context.Background(), edges))

	ctx := &api.TxnContext{}
	txn.FillContext(ctx, 1, true) // isErrored=true: skip txn.Update(), keep the test to Keys
	return ctx.Keys
}

// TestFillContextKeysGolden pins the conflict keys that reach Zero.
func TestFillContextKeysGolden(t *testing.T) {
	// A hardcoded golden is only valid if the keys are startTs-independent.
	// Conflict keys derive from fingerprint(keyBytes) ^ posting.Uid, so they
	// should be — assert it rather than assume it.
	a := fillContextFixture(t, 4_100_000, 0)
	b := fillContextFixture(t, 4_200_000, 0)
	require.Equal(t, a, b, "conflict keys must not depend on startTs")
	require.NotEmpty(t, a, "fixture emitted no conflict keys")

	if len(goldenFillContextKeys) == 0 {
		t.Fatalf("GOLDEN NOT SET — captured %d keys:\n%#v", len(a), a)
	}
	require.Equal(t, goldenFillContextKeys, a,
		"ctx.Keys sent to Zero changed; a SUBSET risks lost updates")

	// The parallel path must emit the same set as the serial path.
	require.Equal(t, a, fillContextFixture(t, 4_300_000, 32),
		"budget>1 must emit identical ctx.Keys")
}
