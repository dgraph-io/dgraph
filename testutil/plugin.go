/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package testutil

import (
	"fmt"
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
		opts = append(opts, "-buildmode=plugin", "-o", so, src)
		cmd := exec.Command("go", opts...)
		cmd.Dir = filepath.Dir(curr)
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
