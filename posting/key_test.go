package posting

import (
	"bytes"
	"testing"
)

func TestDecodeKey(t *testing.T) {
	b := Key(123623, "randomattr")
	uid, attr := DecodeKey(b)

	if attr != "randomattr" {
		t.Errorf("Expected randomattr. Got: %s", attr)
	}
	if uid != 123623 {
		t.Errorf("Expected 123623. Got: %d", uid)
	}
}

func TestDecodeKeyPartial(t *testing.T) {
	b := Key(123623, "randomattr")
	uidB, attr := DecodeKeyPartial(b)

	if attr != "randomattr" {
		t.Errorf("Expected randomattr. Got: %s", attr)
	}

	if len(uidB) != 8 {
		t.Errorf("Wrong number of bytes for uid: %d", len(uidB))
	}

	uid := DecodeUID(uidB)
	if uid != 123623 {
		t.Errorf("Expected 123623. Got: %d", uid)
	}
}

func TestDecodeUID(t *testing.T) {
	if DecodeUID(UID(34657)) != 34657 {
		t.Errorf("Decode . Encode is not identity")
	}
	b := []byte{1, 6, 7, 2, 7, 9, 5, 8}
	if !bytes.Equal(UID(DecodeUID(b)), b) {
		t.Errorf("Encode . Decode is not identity")
	}
}
