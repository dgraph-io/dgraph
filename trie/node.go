package trie

type node interface {
	Encode() ([]byte, error)
}

type (
	branch struct {
		key      []byte // partial key
		children [16]node
		value    []byte
	}
	leaf struct {
		key   []byte // partial key
		value []byte
	}
)

func (b *branch) childrenBitmap() uint16 {
	var bitmap uint16
	var i uint
	for i = 0; i < 16; i++ {
		if b.children[i] != nil {
			bitmap = bitmap | 1<<i
		}
	}
	return bitmap
}

func (b *branch) Encode() ([]byte, error) {
	return nil, nil
}

func (l *leaf) Encode() ([]byte, error) {
	return nil, nil
}
