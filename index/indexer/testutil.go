package indexer

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func arrayCompare(a []string, b []string) error {
	if len(a) != len(b) {
		return fmt.Errorf("Size mismatch %d vs %d", len(a), len(b))
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return fmt.Errorf("Element mismatch at index %d", i)
		}
	}
	return nil
}

func checkQuery(i Indexer, pred, val string, expected []string) error {
	results, err := i.Query(pred, val)
	if err != nil {
		return err
	}
	return arrayCompare(results, expected)
}

func TestBasic(i Indexer, t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = i.Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()

	i.Insert("p1", "k1", "v1")
	if err := checkQuery(i, "p2", "v1", []string{}); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v2", []string{}); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k1"}); err != nil {
		t.Fatal(err)
	}

	if err := i.Insert("p2", "k2", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k1"}); err != nil {
		t.Fatal(err)
	}

	if err := i.Insert("p1", "k2", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k1", "k2"}); err != nil {
		t.Fatal(err)
	}

	if err := i.Insert("p1", "k0", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k0", "k1", "k2"}); err != nil {
		t.Fatal(err)
	}

	// Delete something that is not present.
	if err := i.Delete("p2", "k2", "v5"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k0", "k1", "k2"}); err != nil {
		t.Fatal(err)
	}

	if err := i.Delete("p1", "k1", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k0", "k2"}); err != nil {
		t.Fatal(err)
	}
}

func TestBatch(i Indexer, t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = i.Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()

	b, err := i.NewBatch()
	if err != nil {
		t.Fatal(err)
	}
	err = b.Insert("p1", "k1", "v1")
	if err != nil {
		t.Fatal(err)
	}
	err = b.Insert("p2", "k1", "v1")
	if err != nil {
		t.Fatal(err)
	}
	err = b.Insert("p1", "k2", "v1")
	if err != nil {
		t.Fatal(err)
	}
	err = b.Insert("p1", "k0", "v1")
	if err != nil {
		t.Fatal(err)
	}
	err = b.Delete("p1", "k1", "v1")
	if err != nil {
		t.Fatal(err)
	}

	err = i.Batch(b)
	if err != nil {
		t.Fatal(err)
	}

	err = checkQuery(i, "p1", "v1", []string{"k0", "k2"})
	if err != nil {
		t.Fatal(err)
	}
}
