package main

import (
	"bytes"
	"context"
	"encoding/json"
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
		fmt.Printf("ERROR. Command %q. Error: %v. Output:\n%s\n", command, err, out.String())
		return err
	}
	fmt.Printf("Command %q. Output:\n%s\n", command, out.String())
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
	fmt.Printf("\n===> TIME taken to converge %d alphas: %s\n\n", atLeast, dur)
	return nil
}

func getStatus(zero string) error {
	cmd := exec.Command("http", "GET", fmt.Sprintf("%s/state", zero))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		fmt.Printf("ERROR. Status at %s. Error: %v. Output:\n%s\n", zero, err, out.String())
		return err
	}
	output := out.String()
	if strings.Contains(output, "errors") {
		fmt.Printf("ERROR. Status at %s. Output:\n%s\n", zero, output)
		return fmt.Errorf(output)
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(output), &m); err != nil {
		return err
	}
	pretty, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf("Status at %s:\n%s\n", zero, pretty)
	return nil
}

func testCommon(remove, join string, minAlphasUp int) error {
	var nodes []string
	for i := 1; i <= 3; i++ {
		for j := 1; j <= 3; j++ {
			nodes = append(nodes, fmt.Sprintf("zero%d dg%d", i, j))
		}
	}

	fmt.Printf("Nodes: %+v\n", nodes)
	for _, node := range nodes {
		if err := getStatus("localhost:6080"); err != nil {
			return err
		}
		fmt.Printf("\n==> Remove cmd %q on NODES: %s\n", remove, node)
		if err := run(ctxb, remove+" "+node); err != nil {
			return err
		}
		if err := run(ctxb, "blockade status"); err != nil {
			return err
		}
		if err := increment(minAlphasUp); err != nil {
			return err
		}
		// Then join.
		if err := run(ctxb, join); err != nil {
			return err
		}
		if err := increment(3); err != nil {
			return err
		}
	}
	return nil
}

func waitForHealthy() error {
	for _, zero := range []string{"localhost:6080", "localhost:6082", "localhost:6083"} {
		if err := getStatus(zero); err != nil {
			return err
		}
	}
	for _, alpha := range []string{"localhost:9180", "localhost:9182", "localhost:9183"} {
		if err := run(ctxb, "increment --addr="+alpha); err != nil {
			return err
		}
	}
	return nil
}

func runTests() error {
	defer func() {
		if err := run(ctxb, "blockade destroy"); err != nil {
			log.Fatalf("While destroying: %v", err)
		}
	}()

	for {
		if err := waitForHealthy(); err != nil {
			fmt.Printf("Error while waitForHealthy: %v\n.", err)
			time.Sleep(5 * time.Second)
			fmt.Println("Retrying...")
		} else {
			break
		}
	}

	// Setting flaky --all just does not converge. Too many network interruptions.
	if err := testCommon("blockade flaky", "blockade fast --all", 3); err != nil {
		fmt.Printf("Error testFlaky: %v\n", err)
		return err
	}
	fmt.Println("===> Flaky TEST: OK")

	if err := testCommon("blockade slow", "blockade fast --all", 3); err != nil {
		fmt.Printf("Error testSlow: %v\n", err)
		return err
	}
	fmt.Println("===> Slow TEST: OK")

	if err := testCommon("blockade stop", "blockade start --all", 2); err != nil {
		fmt.Printf("Error testRestart: %v\n", err)
		return err
	}
	fmt.Println("===> Restart TEST: OK")

	if err := testCommon("blockade partition", "blockade join", 2); err != nil {
		fmt.Printf("Error testPartitions: %v\n", err)
		return err
	}
	fmt.Println("===> Partition TEST: OK")

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
