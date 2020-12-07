package main

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	var soFiles []string
	for i, src := range []string{
		"../_customtok/anagram/main.go",
		"../_customtok/cidr/main.go",
		"../_customtok/factor/main.go",
		"../_customtok/rune/main.go",
	} {
		so := strconv.Itoa(i) + ".so"
		fmt.Println("compiling plugin: src=%q so=%q", src, so)
		cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", so, src)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Fatalf("Could not compile plugin: %v", string(out))
		}
		absSO, err := filepath.Abs(so)
		if err != nil {
			log.Fatalf("Could not read plugin file: %v", err)
		}
		soFiles = append(soFiles, absSO)
	}

	log.Print("plugin build completed. Files are: ", strings.Join(soFiles, ","))
}