package babe

import (
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/lib/runtime"
)

func TestInherentExtrinsics(t *testing.T) {
	rt := runtime.NewTestRuntime(t, runtime.POLKADOT_RUNTIME_c768a7e4c70e)

	idata := NewInherentsData()
	err := idata.SetInt64Inherent(Timstap0, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}

	err = idata.SetInt64Inherent(Babeslot, 1)
	if err != nil {
		t.Fatal(err)
	}

	ienc, err := idata.Encode()
	if err != nil {
		t.Fatal(err)
	}

	_, err = rt.InherentExtrinsics(ienc)
	if err != nil {
		t.Fatal(err)
	}
}
