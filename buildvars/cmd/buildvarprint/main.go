/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Command buildvarprint emits the [buildvars] registry as lines of text
// in a selectable format for consumption by shell eval, GNU Make eval,
// or direct parsing by a validation script.
//
// Formats:
//
//	shell (default): export NAME='value'       — for `eval "$(...)"`
//	make:            NAME := value              — for `$(eval $(shell ...))`
//	plain:           NAME=value                 — raw, one per line
//
// Usage:
//
//	eval "$(go run ./buildvars/cmd/buildvarprint)"             # shell
//	go run ./buildvars/cmd/buildvarprint -format=plain         # plain
//	$(eval $(shell go run ./buildvars/cmd/buildvarprint -format=make))
//
// Intended callers:
//
//	make build-env              — uses shell format for eval sourcing
//	istari/scripts/check-buildvars.sh  — uses plain format for diff
//
// Values in shell format are single-quoted with embedded single quotes
// escaped. Values in make format are emitted raw (newlines and special
// Make characters in values would break, but all current buildvars values
// are simple strings).
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dgraph-io/dgraph/v25/buildvars"
)

// shellQuote wraps s in single quotes, escaping any embedded single
// quotes. Matches POSIX shell conventions for literal string quoting:
// 'foo'"'"'bar' unambiguously represents foo'bar.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func main() {
	format := flag.String("format", "shell", "output format: shell | make | plain")
	flag.Parse()

	switch *format {
	case "shell":
		for _, v := range buildvars.All {
			fmt.Fprintf(os.Stdout, "export %s=%s\n", v.Name, shellQuote(v.Get()))
		}
	case "make":
		// `:=` rather than `?=` so buildvarprint's value overrides any
		// ambient Make default. Callers that want fallback-only shape
		// can post-process to replace `:=` with `?=`.
		for _, v := range buildvars.All {
			fmt.Fprintf(os.Stdout, "%s := %s\n", v.Name, v.Get())
		}
	case "plain":
		for _, v := range buildvars.All {
			fmt.Fprintf(os.Stdout, "%s=%s\n", v.Name, v.Get())
		}
	default:
		fmt.Fprintf(os.Stderr, "buildvarprint: unknown format %q (want shell|make|plain)\n", *format)
		os.Exit(2)
	}
}
