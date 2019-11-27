package trie

import (
	"bytes"
	"reflect"
	"testing"
)

func TestPutAndGetChild(t *testing.T) {
	childKey := []byte("default")
	childTrie := buildSmallTrie(t)
	parentTrie := NewEmptyTrie(nil)

	err := parentTrie.PutChild(childKey, childTrie)
	if err != nil {
		t.Fatal(err)
	}

	childTrieRes, err := parentTrie.GetChild(childKey)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(childTrie, childTrieRes) {
		t.Fatalf("Fail: got %v expected %v", childTrieRes, childTrie)
	}
}

func TestPutAndGetFromChild(t *testing.T) {
	childKey := []byte("default")
	childTrie := buildSmallTrie(t)
	parentTrie := NewEmptyTrie(nil)

	err := parentTrie.PutChild(childKey, childTrie)
	if err != nil {
		t.Fatal(err)
	}

	testKey := []byte("child_key")
	testValue := []byte("child_value")
	err = parentTrie.PutIntoChild(childKey, testKey, testValue)
	if err != nil {
		t.Fatal(err)
	}

	valueRes, err := parentTrie.GetFromChild(childKey, testKey)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(valueRes, testValue) {
		t.Fatalf("Fail: got %x expected %x", valueRes, testValue)
	}

	testKey = []byte("child_key_again")
	testValue = []byte("child_value_again")
	err = parentTrie.PutIntoChild(childKey, testKey, testValue)
	if err != nil {
		t.Fatal(err)
	}

	valueRes, err = parentTrie.GetFromChild(childKey, testKey)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(valueRes, testValue) {
		t.Fatalf("Fail: got %x expected %x", valueRes, testValue)
	}
}
