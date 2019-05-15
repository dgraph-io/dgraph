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
	var checkedArgs []string
	for _, arg := range args {
		if len(arg) > 0 {
			checkedArgs = append(checkedArgs, arg)
		}
	}
	cmd := exec.CommandContext(ctx, checkedArgs[0], checkedArgs[1:]...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		fmt.Printf("[%v] ERROR. Command %q. Error: %v. Output:\n%s\n",
			time.Now().UTC(), command, err, out.String())
		return err
	}
	fmt.Printf("[%v] Command %q. Output:\n%s\n", time.Now().UTC(), command, out.String())
	return nil
}

func partition(instance string) error {
	return run(ctxb, fmt.Sprintf("embargo partition %s", instance))
}

func increment(atLeast int, args string) error {
	errCh := make(chan error, 1)
	ctx, cancel := context.WithTimeout(ctxb, 1*time.Minute)
	defer cancel()

	addrs := []string{"localhost:9180", "localhost:9182", "localhost:9183"}
	for _, addr := range addrs {
		go func(addr string) {
			errCh <- run(ctx, fmt.Sprintf("dgraph increment --alpha=%s %s", addr, args))
		}(addr)
	}
	start := time.Now()
	var ok int
	for i := 0; i < len(addrs) && ok < atLeast; i++ {
		if err := <-errCh; err == nil {
			ok++
		} else {
			fmt.Printf("[%v] Got error during increment: %v\n", time.Now().UTC(), err)
		}
	}
	if ok < atLeast {
		return fmt.Errorf("Increment with atLeast=%d failed. OK: %d", atLeast, ok)
	}
	dur := time.Since(start).Round(time.Millisecond)
	fmt.Printf("\n[%v] ===> TIME taken to converge %d alphas: %s\n\n",
		time.Now().UTC(), atLeast, dur)
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
	// var m map[string]interface{}
	// if err := json.Unmarshal([]byte(output), &m); err != nil {
	// 	return err
	// }
	// pretty, err := json.MarshalIndent(m, "", "  ")
	// if err != nil {
	// 	return err
	// }
	fmt.Printf("Status at %s:\n%s\n", zero, output)
	return nil
}

func testCommon(remove, join, incrementArgs string, nodes []string, minAlphasUp int) error {
	fmt.Printf("Nodes: %+v\n", nodes)
	for _, node := range nodes {
		if err := getStatus("localhost:6080"); err != nil {
			return err
		}
		fmt.Printf("\n==> Remove cmd %q on NODES: %s\n", remove, node)
		if err := run(ctxb, remove+" "+node); err != nil {
			return err
		}
		if err := run(ctxb, "embargo status"); err != nil {
			return err
		}
		if err := increment(minAlphasUp, incrementArgs); err != nil {
			return err
		}
		// Then join, if available.
		if len(join) == 0 {
			continue
		}
		if err := run(ctxb, join); err != nil {
			return err
		}
		if err := increment(3, incrementArgs); err != nil {
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
		if err := run(ctxb, "dgraph increment --alpha="+alpha); err != nil {
			return err
		}
	}
	return nil
}

func runTests() error {
	for {
		if err := waitForHealthy(); err != nil {
			fmt.Printf("Error while waitForHealthy: %v\n.", err)
			time.Sleep(5 * time.Second)
			fmt.Println("Retrying...")
		} else {
			break
		}
	}

	var nodes []string
	for i := 1; i <= 3; i++ {
		for j := 1; j <= 3; j++ {
			nodes = append(nodes, fmt.Sprintf("zero%d dg%d", i, j))
		}
	}

	var alphaNodes []string
	for i := 1; i <= 3; i++ {
		alphaNodes = append(alphaNodes, fmt.Sprintf("dg%d", i))
	}

	// Setting flaky --all just does not converge. Too many network interruptions.
	// if err := testCommon("embargo flaky", "embargo fast --all", 3); err != nil {
	// 	fmt.Printf("Error testFlaky: %v\n", err)
	// 	return err
	// }
	// fmt.Println("===> Flaky TEST: OK")

	// if err := testCommon("embargo slow", "embargo fast --all", 3); err != nil {
	// 	fmt.Printf("Error testSlow: %v\n", err)
	// 	return err
	// }
	// fmt.Println("===> Slow TEST: OK")

	if err := testCommon("embargo stop", "embargo start --all", "", nodes, 2); err != nil {
		fmt.Printf("Error testStop: %v\n", err)
		return err
	}
	fmt.Println("===> Stop TEST: OK")

	if err := testCommon("embargo restart", "", "", nodes, 3); err != nil {
		fmt.Printf("Error testRestart with restart: %v\n", err)
		return err
	}
	fmt.Println("===> Restart TEST: OK")

	if err := testCommon("embargo partition", "embargo join", "", nodes, 2); err != nil {
		fmt.Printf("Error testPartitions: %v\n", err)
		return err
	}
	fmt.Println("===> Partition TEST: OK")

	if err := testCommon("embargo partition", "embargo join", "--be", alphaNodes, 3); err != nil {
		fmt.Printf("Error testPartitionsBestEffort: %v\n", err)
		return err
	}
	fmt.Println("===> Partition best-effort TEST: OK")

	return nil
}

func main() {
	rand.Seed(time.Now().UnixNano())
	fmt.Println("Starting embargo")
	if err := run(ctxb, "embargo up"); err != nil {
		log.Fatal(err)
	}
	// This defer can be moved within runTests, if we want to destroy embargo,
	// in case our tests fail. We don't want to do that, because then we won't
	// be able to get the logs.
	defer func() {
		if err := run(ctxb, "embargo destroy"); err != nil {
			log.Fatalf("While destroying: %v", err)
		}
	}()

	if err := runTests(); err != nil {
		os.Exit(1)
	}
	fmt.Println("embargo tests: OK")
}
