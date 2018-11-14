package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"
)

var ctxb = context.Background()

func run(ctx context.Context, command string) error {
	args := strings.Split(command, " ")
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		fmt.Printf("ERROR. Command %s. Error: %v. Output:\n%s\n", command, err, out.String())
		return err
	}
	fmt.Printf("Command %s. Output:\n%s\n", command, out.String())
	return nil
}

func partition(instance string) error {
	return run(ctxb, fmt.Sprintf("blockade partition %s", instance))
}

func increment(atLeast int) error {
	errCh := make(chan error, 1)
	ctx, cancel := context.WithTimeout(ctxb, time.Minute)
	defer cancel()

	addrs := []string{"localhost:9180", "localhost:9182", "localhost:9183"}
	for _, addr := range addrs {
		go func(addr string) {
			errCh <- run(ctx, fmt.Sprintf("increment --addr=%s", addr))
		}(addr)
	}
	start := time.Now()
	for i := 0; i < atLeast; i++ {
		if err := <-errCh; err != nil {
			fmt.Printf("Got error during increment: %v\n", err)
			return err
		}
	}
	dur := time.Since(start).Round(time.Millisecond)
	fmt.Printf("Time taken to converge %d: %s\n", atLeast, dur)
	return nil
}

func testPartitions() error {
	var nodes []string
	for i := 1; i <= 3; i++ {
		for j := 1; j <= 3; j++ {
			nodes = append(nodes, fmt.Sprintf("zero%d dg%d", i, j))
		}
	}

	fmt.Printf("Nodes: %v\n", nodes)
	for _, node := range nodes {
		// First partition.
		if err := run(ctxb, "http GET localhost:6080/state"); err != nil {
			return err
		}
		fmt.Printf("\n==> Partitioning NODE: %s\n", node)
		if err := partition(node); err != nil {
			return err
		}
		if err := run(ctxb, "blockade status"); err != nil {
			return err
		}
		if err := increment(2); err != nil {
			return err
		}
		// Then join.
		if err := run(ctxb, "blockade join"); err != nil {
			return err
		}
		if err := increment(3); err != nil {
			return err
		}
	}
	fmt.Println("testPartitions: OK")
	return nil
}

func runTests() error {
	defer func() {
		if err := run(ctxb, "blockade destroy"); err != nil {
			log.Fatalf("While destroying: %v", err)
		}
	}()

	if err := run(ctxb,
		"increment --addr=localhost:9180"); err != nil {
		fmt.Printf("Error during increment: %v\n", err)
		return err
	}
	if err := testPartitions(); err != nil {
		fmt.Printf("Error testPartitions: %v\n", err)
		return err
	}
	return nil
}

func main() {
	rand.Seed(time.Now().UnixNano())
	fmt.Println("Starting blockade")
	if err := run(ctxb, "blockade up"); err != nil {
		log.Fatal(err)
	}
	if err := runTests(); err != nil {
		os.Exit(1)
	}
	fmt.Println("Blockade tests: OK")
}
