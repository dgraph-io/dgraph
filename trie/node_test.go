package trie

import (
	"testing"
)

func TestChildrenBitmap(t *testing.T) {
	b := &branch{children: [16]node{}}
	res := b.childrenBitmap()
	if res != 0 {
		t.Errorf("Fail to get children bitmap: got %x expected %x", res, 1)
	}

	b.children[0] = &leaf{key: []byte{0x00}, value: []byte{0x00}}
	res = b.childrenBitmap()
	if res != 1 {
		t.Errorf("Fail to get children bitmap: got %x expected %x", res, 1)
	}

	b.children[4] = &leaf{key: []byte{0x00}, value: []byte{0x00}}
	res = b.childrenBitmap()
	if res != 1<<4+1 {
		t.Errorf("Fail to get children bitmap: got %x expected %x", res, 17)
	}

	b.children[15] = &leaf{key: []byte{0x00}, value: []byte{0x00}}
	res = b.childrenBitmap()
	if res != 1<<15+1<<4+1 {
		t.Errorf("Fail to get children bitmap: got %x expected %x", res, 257)
	}
}
