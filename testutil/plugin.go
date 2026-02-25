/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func GeneratePlugins(raceEnabled bool) {
	_, curr, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Print("error while getting current file")
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
		opts := []string{"build"}
		if raceEnabled {
			opts = append(opts, "-race")
		}
		opts = append(opts, "-buildmode=plugin")
		if runtime.GOOS != "linux" {
			// Use the BFD linker; the default gold linker is not shipped
			// with most cross-compiler toolchains.
			opts = append(opts, "-ldflags", "-extldflags -fuse-ld=bfd")
		}
		opts = append(opts, "-o", so, src)
		cmd := exec.Command("go", opts...)
		cmd.Dir = filepath.Dir(curr)
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH)
		if runtime.GOOS != "linux" {
			cmd.Env = append(cmd.Env, "CGO_ENABLED=1", "CC="+linuxCC())
		}
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

// linuxCC returns the C cross-compiler for targeting Linux from the current host.
// Respects the LINUX_CC environment variable if set.
func linuxCC() string {
	if cc := os.Getenv("LINUX_CC"); cc != "" {
		return cc
	}
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64-unknown-linux-gnu-gcc"
	case "amd64":
		return "x86_64-unknown-linux-gnu-gcc"
	default:
		return "gcc"
	}
}
