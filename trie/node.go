package trie

type node interface {
	// TODO, will be filled when we implement caching and hashing
}

type (
	branch struct {
		children [17]node
	}
	extension struct {
		key   []byte // partial key
		value node   // child node
	}
	leaf []byte
)
