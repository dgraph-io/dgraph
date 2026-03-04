/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package conn

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnappyCompressDecompressRoundTrip(t *testing.T) {
	c := snappyCompressor{}
	original := []byte("Hello, this is a test of snappy compression round trip!")

	// Compress.
	var compressed bytes.Buffer
	w, err := c.Compress(&compressed)
	require.NoError(t, err)
	_, err = w.Write(original)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Decompress.
	r, err := c.Decompress(&compressed)
	require.NoError(t, err)
	decompressed, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, original, decompressed)
}

func TestSnappyCompressorName(t *testing.T) {
	c := snappyCompressor{}
	require.Equal(t, "snappy", c.Name())
}

func TestSnappyConcurrentCompressDecompress(t *testing.T) {
	c := snappyCompressor{}
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			original := []byte("concurrent test data for snappy compression")

			// Compress.
			var compressed bytes.Buffer
			w, err := c.Compress(&compressed)
			require.NoError(t, err)
			_, err = w.Write(original)
			require.NoError(t, err)
			require.NoError(t, w.Close())

			// Decompress.
			r, err := c.Decompress(&compressed)
			require.NoError(t, err)
			decompressed, err := io.ReadAll(r)
			require.NoError(t, err)
			require.Equal(t, original, decompressed)
		}(i)
	}

	wg.Wait()
}

func TestSnappyEmptyData(t *testing.T) {
	c := snappyCompressor{}

	// Compress empty data.
	var compressed bytes.Buffer
	w, err := c.Compress(&compressed)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Decompress empty data.
	r, err := c.Decompress(&compressed)
	require.NoError(t, err)
	decompressed, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Empty(t, decompressed)
}
