// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

// +build none

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	GOLANGCI_VERSION = "github.com/golangci/golangci-lint/cmd/golangci-lint@v1.23.1"
)

func main() {

	log.SetFlags(log.Lshortfile)

	if _, err := os.Stat(filepath.Join("scripts", "ci.go")); os.IsNotExist(err) {
		log.Fatal("should run build from root dir")
	}
	if len(os.Args) < 2 {
		log.Fatal("cmd required, eg: install")
	}
	switch os.Args[1] {
	case "install":
		install(false)
	case "install-debug":
		install(true)
	case "lint":
		lint()
	case "test":
		test()
	default:
		log.Fatal("cmd not found", os.Args[1])
	}
}

func install(debug bool) {
	argsList := append([]string{"list"}, []string{"./..."}...)

	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), argsList...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("could not list packages: %v\n%s", err, string(out))
	}
	var packages []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "/gossamer/") {
			packages = append(packages, strings.TrimSpace(line))
		}
	}

	argsInstall := append([]string{"install"})
	cmd = exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), argsInstall...)
	cmd.Args = append(cmd.Args, "-v")
	if debug {
		cmd.Args = append(cmd.Args, "-gcflags=\"all=-N -l\"")
	}
	cmd.Args = append(cmd.Args, packages...)

	fmt.Println("Build Gossamer", strings.Join(cmd.Args, " \\\n"))
	cmd.Stderr, cmd.Stdout = os.Stderr, os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatal("Error: Could not build Gossamer. ", "error: ", err, ", cmd: ", cmd)
	}

}

func lint() {

	verbose := flag.Bool("v", false, "Whether to log verbosely")

	// Make sure golangci-lint is available
	argsGet := append([]string{"get", GOLANGCI_VERSION})
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), argsGet...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("could not list packages: %v\n%s", err, string(out))
	}

	cmd = exec.Command(filepath.Join(GOBIN(), "golangci-lint"))
	cmd.Args = append(cmd.Args, "run", "--config", ".golangci.yml")

	if *verbose {
		cmd.Args = append(cmd.Args, "-v")
	}

	fmt.Println("Lint Gossamer", strings.Join(cmd.Args, " \\\n"))
	cmd.Stderr, cmd.Stdout = os.Stderr, os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatal("Error: Could not Lint Gossamer. ", "error: ", err, ", cmd: ", cmd)
	}
}

func test() {

	verbose := flag.Bool("v", false, "Whether to log verbosely")

	packages := []string{"./..."}
	argsInstall := append([]string{"test"})
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), argsInstall...)
	cmd.Args = append(cmd.Args, "-p", "1", "-timeout", "5m")

	if *verbose {
		cmd.Args = append(cmd.Args, "-v")
	}

	cmd.Args = append(cmd.Args, packages...)

	fmt.Println("Test Gossamer", strings.Join(cmd.Args, " \\\n"))
	cmd.Stderr, cmd.Stdout = os.Stderr, os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatal("Error: Could not test Gossamer. ", "error: ", err, ", cmd: ", cmd)
	}
}

// GOBIN returns the GOBIN environment variable
func GOBIN() string {
	if os.Getenv("GOBIN") == "" {
		log.Fatal("GOBIN is not set")
	}
	return os.Getenv("GOBIN")
}
