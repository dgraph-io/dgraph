package main

import (
	"bufio"
	"flag"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
)

func TestQuery(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	dir, err := ioutil.TempDir("", "storetest_")
	dir1, err1 := ioutil.TempDir("", "storetest1_")
	if err != nil || err1 != nil {
		t.Fail()
	}
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir1)

	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		t.Fail()
	}

	ps1, err := store.NewStore(dir1)
	if err != nil {
		t.Error(err)
		t.Fail()
	}

	clog := commit.NewLogger(dir, "mutations", 50<<20)
	clog.Init()
	defer clog.Close()
	posting.Init(clog)

	uid.Init(ps)
	loader.Init(ps, ps1)
	posting.InitIndex(ps1)

	var count uint64
	{
		f, err := os.Open("test_input")
		if err != nil {
			t.Error(err)
			t.Fail()
		}
		r := bufio.NewReader(f)
		count, err = loader.AssignUids(r, 0, 1) // Assign uids for everything.
		t.Logf("count: %v", count)
		f.Close()
		posting.MergeLists(100)
	}
	{
		f, err := os.Open("test_input")
		if err != nil {
			t.Error(err)
			t.Fail()
		}
		r := bufio.NewReader(f)
		count, err = loader.LoadEdges(r, 1, 2)
		t.Logf("count: %v", count)
		f.Close()
		posting.MergeLists(100)
	}

	if farm.Fingerprint64([]byte("follows"))%2 != 1 {
		t.Error("Expected fp to be 1.")
		t.Fail()
	}
	if count != 4 {
		t.Error("loader assignment not as expected")
	}

	{
		f, err := os.Open("test_input")
		if err != nil {
			t.Error(err)
			t.Fail()
		}
		r := bufio.NewReader(f)
		count, err = loader.LoadEdges(r, 0, 2)
		t.Logf("count: %v", count)
		f.Close()
		posting.MergeLists(100)
	}

	if farm.Fingerprint64([]byte("enemy"))%2 != 0 {
		t.Error("Expected fp to be 0.")
		t.Fail()
	}
	if count != 4 {
		t.Error("loader assignment not as expected")
	}
}

var uiddir = flag.String("uid", "", "UID directory")
var rdffile = flag.String("rdf", "", "RDF file")

func BenchmarkLoadRW(b *testing.B) {
	flag.Parse()
	logrus.SetLevel(logrus.ErrorLevel)
	var nameL []string

	uidStore, err := store.NewStore(*uiddir)
	if err != nil {
		b.Errorf("Error creating uidStore: %v", err)
		return
	}
	defer uidStore.Close()

	posting.Init(nil)
	uid.Init(uidStore)

	f, err := os.Open("nameslist")
	if err != nil {
		b.Error("Error opening file")
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		nameL = append(nameL, scanner.Text())
	}

	lenL := len(nameL)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it := nameL[rand.Intn(lenL)]
		uidStore.Get(uid.StringKey(it))
	}
}

func BenchmarkLoadReadOnly(b *testing.B) {
	flag.Parse()
	logrus.SetLevel(logrus.ErrorLevel)
	var nameL []string

	uidStore, err := store.NewReadOnlyStore(*uiddir)
	if err != nil {
		b.Errorf("Error creating uidStore: %v", err)
		return
	}
	defer uidStore.Close()

	posting.Init(nil)
	uid.Init(uidStore)

	f, err := os.Open("nameslist")
	if err != nil {
		b.Error("Error opening file")
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		nameL = append(nameL, scanner.Text())
	}

	lenL := len(nameL)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it := nameL[rand.Intn(lenL)]
		uidStore.Get(uid.StringKey(it))
	}
}
