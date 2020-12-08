package testutil

import (
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func GeneratePlugins() {
	_, curr, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Print("error while getting current directory")
		return
	}
	var soFiles []string
	for i, src := range []string{
		"./custom_plugins/anagram/main.go",
		"./custom_plugins/cidr/main.go",
		"./custom_plugins/factor/main.go",
		"./custom_plugins/rune/main.go",
	} {
		so := "./custom_plugins/" + strconv.Itoa(i) + ".so"
		fmt.Printf("compiling plugin: src=%q so=%q\n", src, so)
		cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", so, src)
		cmd.Dir = path.Dir(curr)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("Error: %v\n", err)
			fmt.Printf("Output: %v\n", string(out))
			return
		}
		absSO, err := filepath.Abs(so)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		soFiles = append(soFiles, absSO)
	}

	fmt.Printf("plugin build completed. Files are: %s\n", strings.Join(soFiles, ","))
}