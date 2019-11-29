// +build none

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
		install()
	default:
		log.Fatal("cmd not found", os.Args[1])
	}
}

func install() {

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
	cmd.Args = append(cmd.Args, packages...)

	fmt.Println("Build Gossamer", strings.Join(cmd.Args, " \\\n"))
	cmd.Stderr, cmd.Stdout = os.Stderr, os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatal("Error: Could not build Gossamer. ", "error: ", err, ", cmd: ", cmd)
	}

}
