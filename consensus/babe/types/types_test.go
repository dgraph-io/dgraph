package types

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/crypto/sr25519"
)

func TestBabeHeader_EncodeAndDecode(t *testing.T) {
	// arbitrary test data
	bh := &BabeHeader{
		VrfOutput:          [sr25519.VrfOutputLength]byte{0, 91, 50, 25, 214, 94, 119, 36, 71, 216, 33, 152, 85, 184, 34, 120, 61, 161, 164, 223, 76, 53, 40, 246, 76, 38, 235, 204, 43, 31, 179, 28},
		VrfProof:           [sr25519.VrfProofLength]byte{120, 23, 235, 159, 115, 122, 207, 206, 123, 232, 75, 243, 115, 255, 131, 181, 219, 241, 200, 206, 21, 22, 238, 16, 68, 49, 86, 99, 76, 139, 39, 0, 102, 106, 181, 136, 97, 141, 187, 1, 234, 183, 241, 28, 27, 229, 133, 8, 32, 246, 245, 206, 199, 142, 134, 124, 226, 217, 95, 30, 176, 246, 5, 3},
		BlockProducerIndex: 17,
		SlotNumber:         420,
	}

	enc := bh.Encode()
	bh2 := new(BabeHeader)
	err := bh2.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(bh, bh2) {
		t.Fatalf("Fail: got %v expected %v", bh2, bh)
	}
}
