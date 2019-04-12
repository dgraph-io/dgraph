package trie

import (
	"fmt"
)

// Print prints the trie through pre-order traversal
func (t *Trie) Print() {
	fmt.Println("printing trie...")
	t.print(t.root)
}

func (t *Trie) print(current node) {
	switch c := current.(type) {
	case *branch:
		fmt.Printf("branch pk %x children %b value %s\n", c.key, c.childrenBitmap(), c.value)
		for _, child := range c.children {
			t.print(child)
		}
	case *leaf:
		fmt.Printf("leaf pk %x val %s\n", c.key, c.value)
	default:
		// do nothing
	}
}
