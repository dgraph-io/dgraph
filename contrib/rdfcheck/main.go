/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 *
 * rdfcheck - Validate RDF files using Dgraph's parser
 *
 * Usage:
 *   rdfcheck [flags] <file1.rdf> [file2.rdf] ...
 *   cat file.rdf | rdfcheck -
 *
 * Flags:
 *   -v, --verbose    Print each error (default: summary only)
 *   -s, --stop       Stop on first error
 *   -c, --count      Print count of valid NQuads per file
 */
package main

import (
	"bufio"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/dgraph/v25/chunker"
	"github.com/dgraph-io/dgraph/v25/lex"
)

var (
	verbose   bool
	stopFirst bool
	countOnly bool
	parallel  int
)

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Print each error with line number and content")
	flag.BoolVar(&stopFirst, "stop", false, "Stop on first error")
	flag.BoolVar(&countOnly, "count", false, "Print count of valid NQuads per file")
	flag.IntVar(&parallel, "j", 1, "Number of files to process in parallel")
}

type fileResult struct {
	filename   string
	nquadCount int64
	errorCount int64
	errors     []string
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "rdfcheck - Validate RDF files using Dgraph's parser\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  rdfcheck [flags] <file1.rdf> [file2.rdf] ...\n")
		fmt.Fprintf(os.Stderr, "  cat file.rdf | rdfcheck -\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	files := flag.Args()

	// Handle stdin
	if len(files) == 1 && files[0] == "-" {
		result := processReader(os.Stdin, "<stdin>")
		printResult(result)
		if result.errorCount > 0 {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Process files
	var totalErrors int64
	var totalNquads int64

	if parallel <= 1 {
		// Sequential processing
		for _, file := range files {
			result := processFile(file)
			printResult(result)
			totalErrors += result.errorCount
			totalNquads += result.nquadCount
			if stopFirst && result.errorCount > 0 {
				break
			}
		}
	} else {
		// Parallel processing
		results := make(chan fileResult, len(files))
		sem := make(chan struct{}, parallel)
		var wg sync.WaitGroup

		for _, file := range files {
			wg.Add(1)
			go func(f string) {
				defer wg.Done()
				sem <- struct{}{}
				results <- processFile(f)
				<-sem
			}(file)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		for result := range results {
			printResult(result)
			totalErrors += result.errorCount
			totalNquads += result.nquadCount
		}
	}

	// Summary
	if len(files) > 1 || countOnly {
		fmt.Printf("\n=== Summary ===\n")
		fmt.Printf("Files processed: %d\n", len(files))
		fmt.Printf("Total NQuads: %d\n", totalNquads)
		fmt.Printf("Total Errors: %d\n", totalErrors)
	}

	if totalErrors > 0 {
		os.Exit(1)
	}
}

func processFile(filename string) fileResult {
	f, err := os.Open(filename)
	if err != nil {
		return fileResult{
			filename:   filename,
			errorCount: 1,
			errors:     []string{fmt.Sprintf("failed to open file: %v", err)},
		}
	}
	defer f.Close()

	var reader io.Reader = f

	// Handle gzipped files
	if strings.HasSuffix(strings.ToLower(filename), ".gz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return fileResult{
				filename:   filename,
				errorCount: 1,
				errors:     []string{fmt.Sprintf("failed to create gzip reader: %v", err)},
			}
		}
		defer gzr.Close()
		reader = gzr
	}

	return processReader(reader, filename)
}

func processReader(reader io.Reader, filename string) fileResult {
	result := fileResult{filename: filename}

	scanner := bufio.NewScanner(reader)
	// Increase buffer size for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // 10MB max line length

	var lineNum int64
	var l lex.Lexer

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines and comments early
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, "#") {
			continue
		}

		_, err := chunker.ParseRDF(line, &l)
		if err != nil {
			if errors.Is(err, chunker.ErrEmpty) {
				continue
			}

			atomic.AddInt64(&result.errorCount, 1)

			if verbose {
				errMsg := fmt.Sprintf("line %d: %v", lineNum, err)
				if len(line) > 200 {
					errMsg += fmt.Sprintf("\n  content: %s...", line[:200])
				} else {
					errMsg += fmt.Sprintf("\n  content: %s", line)
				}
				result.errors = append(result.errors, errMsg)
			}

			if stopFirst {
				return result
			}
		} else {
			atomic.AddInt64(&result.nquadCount, 1)
		}
	}

	if err := scanner.Err(); err != nil {
		result.errorCount++
		result.errors = append(result.errors, fmt.Sprintf("scanner error: %v", err))
	}

	return result
}

func printResult(result fileResult) {
	basename := filepath.Base(result.filename)

	if result.errorCount == 0 {
		if countOnly || verbose {
			fmt.Printf("✓ %s: %d NQuads, 0 errors\n", basename, result.nquadCount)
		}
	} else {
		fmt.Printf("✗ %s: %d NQuads, %d errors\n", basename, result.nquadCount, result.errorCount)
		if verbose {
			for _, e := range result.errors {
				fmt.Printf("  %s\n", e)
			}
		}
	}
}
