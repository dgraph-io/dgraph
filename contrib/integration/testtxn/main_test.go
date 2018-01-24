package main_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type state struct {
	Commands []*exec.Cmd
	Dirs     []string
	dg       *client.Dgraph
}

var s state

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	cmd := exec.Command("go", "install", "github.com/dgraph-io/dgraph/dgraph")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("Could not run %q: %s", cmd.Args, string(out))
	}

	zero := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "zero", "-w=wz")
	zero.Stdout = os.Stdout
	zero.Stderr = os.Stderr
	if err := zero.Start(); err != nil {
		log.Fatal(err)
	}
	s.Dirs = append(s.Dirs, "wz")
	s.Commands = append(s.Commands, zero)

	time.Sleep(5 * time.Second)
	dgraph := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"server",
		"--memory_mb=2048",
		fmt.Sprintf("--zero=127.0.0.1:%d", x.PortZeroGrpc),
		"-o=1",
	)
	dgraph.Stdout = os.Stdout
	dgraph.Stderr = os.Stderr

	if err := dgraph.Start(); err != nil {
		log.Fatal(err)
	}
	time.Sleep(5 * time.Second)

	s.Commands = append(s.Commands, dgraph)
	s.Dirs = append(s.Dirs, "p", "w")

	conn, err := grpc.Dial("localhost:9081", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := api.NewDgraphClient(conn)

	dg := client.NewDgraphClient(dc)
	s.dg = dg
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			s.dg.NewTxn()
			wg.Done()
		}()
	}
	wg.Wait()

	op := &api.Operation{}
	op.Schema = `name: string @index(fulltext) .`
	if err := s.dg.Alter(context.Background(), op); err != nil {
		log.Fatal(err)
	}

	r := m.Run()
	for _, cmd := range s.Commands {
		cmd.Process.Kill()
	}
	for _, dir := range s.Dirs {
		os.RemoveAll(dir)
	}
	os.Exit(r)
}

// TODO - Cleanup this file so that it is more in sync with how other tests are written.
// readTs == startTs
func TestTxnRead1(t *testing.T) {
	fmt.Println("TestTxnRead1")
	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
	require.NoError(t, txn.Commit(context.Background()))
}

// readTs < commitTs
func TestTxnRead2(t *testing.T) {
	fmt.Println("TestTxnRead2")
	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	txn2 := s.dg.NewTxn()

	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn2.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTruef(bytes.Equal(resp.Json, []byte("{\"me\":[]}")), "%s", resp.Json)
	require.NoError(t, txn.Commit(context.Background()))
}

// readTs > commitTs
func TestTxnRead3(t *testing.T) {
	op := &api.Operation{}
	op.DropAttr = "name"
	attempts := 0
	for attempts < 10 {
		if err := s.dg.Alter(context.Background(), op); err == nil {
			break
		}
		attempts++
	}

	fmt.Println("TestTxnRead3")
	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	require.NoError(t, txn.Commit(context.Background()))
	txn = s.dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
}

// readTs > commitTs
func TestTxnRead4(t *testing.T) {
	fmt.Println("TestTxnRead4")
	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	require.NoError(t, txn.Commit(context.Background()))
	txn2 := s.dg.NewTxn()

	txn3 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Manish2"}`, uid))
	fmt.Println(string(mu.SetJson))
	assigned, err = txn3.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn2.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))

	fmt.Println("Committing txn3")
	require.NoError(t, txn3.Commit(context.Background()))

	txn4 := s.dg.NewTxn()
	q = fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err = txn4.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish2\"}]}")))
}

func TestTxnRead5(t *testing.T) {
	fmt.Println("TestTxnRead5")
	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	require.NoError(t, txn.Commit(context.Background()))
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	// We don't supply startTs, it should be fetched from zero by dgraph server.
	req := api.Request{
		Query: q,
	}

	conn, err := grpc.Dial("localhost:9081", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := api.NewDgraphClient(conn)

	resp, err := dc.Query(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
	x.AssertTrue(resp.Txn.StartTs > 0)

	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf("{\"uid\": \"%s\", \"name\": \"Manish2\"}", uid))

	mu.CommitNow = true
	res, err := dc.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	x.AssertTrue(res.Context.StartTs > 0)
	resp, err = dc.Query(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte(`{"me":[{"name":"Manish2"}]}`)))
}

func TestConflict(t *testing.T) {
	fmt.Println("TestConflict")
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	txn2 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Manish"}`, uid))
	x.Check2(txn2.Mutate(context.Background(), mu))

	require.NoError(t, txn.Commit(context.Background()))
	err = txn2.Commit(context.Background())
	x.AssertTrue(err != nil)

	txn = s.dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
}

func TestConflictTimeout(t *testing.T) {
	fmt.Println("TestConflictTimeout")
	var uid string
	txn := s.dg.NewTxn()
	{
		mu := &api.Mutation{}
		mu.SetJson = []byte(`{"name": "Manish"}`)
		assigned, err := txn.Mutate(context.Background(), mu)
		if err != nil {
			log.Fatalf("Error while running mutation: %v\n", err)
		}
		if len(assigned.Uids) != 1 {
			log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
		}
		for _, u := range assigned.Uids {
			uid = u
		}
	}

	txn2 := s.dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn2.Query(context.Background(), q)
	require.NoError(t, err)
	fmt.Printf("Response should be empty. JSON: %q\n", resp.Json)

	mu := &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Jan the man"}`, uid))
	_, err = txn2.Mutate(context.Background(), mu)
	fmt.Printf("txn2.mutate error: %v\n", err)
	if err == nil {
		require.NoError(t, txn2.Commit(context.Background()))
	}

	err = txn.Commit(context.Background())
	fmt.Printf("This txn should fail with error. Err got: %v\n", err)
	x.AssertTrue(err != nil)

	txn3 := s.dg.NewTxn()
	q = fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err = txn3.Query(context.Background(), q)
	require.NoError(t, err)
	fmt.Printf("Final Response JSON: %q\n", resp.Json)
}

func TestConflictTimeout2(t *testing.T) {
	fmt.Println("TestConflictTimeout2")
	var uid string
	txn := s.dg.NewTxn()
	{

		mu := &api.Mutation{}
		mu.SetJson = []byte(`{"name": "Manish"}`)
		assigned, err := txn.Mutate(context.Background(), mu)
		if err != nil {
			log.Fatalf("Error while running mutation: %v\n", err)
		}
		if len(assigned.Uids) != 1 {
			log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
		}
		for _, u := range assigned.Uids {
			uid = u
		}
	}

	txn2 := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Jan the man"}`, uid))
	x.Check2(txn2.Mutate(context.Background(), mu))

	require.NoError(t, txn.Commit(context.Background()))
	err := txn2.Commit(context.Background())
	x.AssertTrue(err != nil)
	fmt.Printf("This txn commit should fail with error. Err got: %v\n", err)

	txn3 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Jan the man"}`, uid))
	assigned, err := txn3.Mutate(context.Background(), mu)
	fmt.Printf("txn2.mutate error: %v\n", err)
	if err == nil {
		require.NoError(t, txn3.Commit(context.Background()))
	}
	for _, u := range assigned.Uids {
		uid = u
	}

	txn4 := s.dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn4.Query(context.Background(), q)
	require.NoError(t, err)
	fmt.Printf("Final Response JSON: %q\n", resp.Json)
}

func TestIgnoreIndexConflict(t *testing.T) {
	fmt.Println("TestConflict")
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `name: string @index(exact) .`
	if err := s.dg.Alter(context.Background(), op); err != nil {
		log.Fatal(err)
	}

	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	mu.IgnoreIndexConflict = true
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid1, uid2 string
	for _, u := range assigned.Uids {
		uid1 = u
	}

	txn2 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.IgnoreIndexConflict = true
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err = txn2.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	for _, u := range assigned.Uids {
		uid2 = u
	}

	require.NoError(t, txn.Commit(context.Background()))
	require.NoError(t, txn2.Commit(context.Background()))

	txn = s.dg.NewTxn()
	q := `{ me(func: eq(name, "Manish")) { uid }}`
	resp, err := txn.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	expectedResp := []byte(fmt.Sprintf(`{"me":[{"uid":"%s"},{"uid":"%s"}]}`, uid1, uid2))
	fmt.Printf("Response JSON: %q, Expected JSON: %q\n", resp.Json, expectedResp)
	x.AssertTrue(bytes.Equal(resp.Json, expectedResp))
}

func TestReadIndexKeySameTxn(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `name: string @index(exact) .`
	if err := s.dg.Alter(context.Background(), op); err != nil {
		log.Fatal(err)
	}

	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	mu.IgnoreIndexConflict = true
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	q := `{ me(func: le(name, "Manish")) { uid }}`
	resp, err := txn.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	expectedResp := []byte(fmt.Sprintf(`{"me":[{"uid":"%s"}]}`, uid))
	fmt.Printf("Response JSON: %q, Expected JSON: %q\n", resp.Json, expectedResp)
	x.AssertTrue(bytes.Equal(resp.Json, expectedResp))
}

func TestSPStar(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `friend: uid .`
	require.NoError(t, s.dg.Alter(context.Background(), op))

	txn := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish", "friend": [{"name": "Jan"}]}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	uid1 := assigned.Uids["blank-0"]
	require.NoError(t, err)
	require.Equal(t, 2, len(assigned.Uids))
	require.NoError(t, txn.Commit(context.Background()))

	txn = s.dg.NewTxn()
	mu = &api.Mutation{}
	client.DeleteEdges(mu, uid1, "friend")
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 0, len(assigned.Uids))

	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s" ,"name": "Manish", "friend": [{"name": "Jan2"}]}`, uid1))
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 1, len(assigned.Uids))
	uid2 := assigned.Uids["blank-0"]

	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			uid
			friend {
				uid
				name
			}
		}
	}`, uid1)

	resp, err := txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp := fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan2", "uid":"%s"}]}]}`, uid1, uid2)
	require.JSONEq(t, expectedResp, string(resp.Json))
}

func TestSPStar2(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `friend: uid .`
	require.NoError(t, s.dg.Alter(context.Background(), op))

	// Add edge
	txn := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish", "friend": [{"name": "Jan"}]}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	uid1 := assigned.Uids["blank-0"]
	uid2 := assigned.Uids["blank-1"]
	require.NoError(t, err)
	require.Equal(t, 2, len(assigned.Uids))

	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			uid
			friend {
				uid
				name
			}
		}
	}`, uid1)

	resp, err := txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp := fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan", "uid":"%s"}]}]}`, uid1, uid2)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Delete S P *
	mu = &api.Mutation{}
	client.DeleteEdges(mu, uid1, "friend")
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 0, len(assigned.Uids))

	resp, err = txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s"}]}`, uid1)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Add edge
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s" ,"name": "Manish", "friend": [{"name": "Jan2"}]}`, uid1))
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 1, len(assigned.Uids))
	uid2 = assigned.Uids["blank-0"]

	resp, err = txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan2", "uid":"%s"}]}]}`, uid1, uid2)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Delete S P *
	mu = &api.Mutation{}
	client.DeleteEdges(mu, uid1, "friend")
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 0, len(assigned.Uids))

	resp, err = txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s"}]}`, uid1)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Add edge
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s" ,"name": "Manish", "friend": [{"name": "Jan3"}]}`, uid1))
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 1, len(assigned.Uids))
	uid2 = assigned.Uids["blank-0"]

	resp, err = txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan3", "uid":"%s"}]}]}`, uid1, uid2)
	require.JSONEq(t, expectedResp, string(resp.Json))
}
