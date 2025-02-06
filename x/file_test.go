/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindDataFiles(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := [2]string{"test-files-valid", "test-files-invalid"}
	require.NoError(t, os.MkdirAll(dir[0], os.ModePerm))
	require.NoError(t, os.MkdirAll(dir[1], os.ModePerm))
	defer deleteDirs(t, dir[0])
	defer deleteDirs(t, dir[1])
	validTestFiles := filepath.Join(filepath.Dir(thisFile), "test-files-valid")
	invalidTestFiles := filepath.Join(filepath.Dir(thisFile), "test-files-invalid")
	expectedFilesArray := []string{
		"test-files-valid/test-2.rdf.gz",
		"test-files-valid/test-3.json",
		"test-files-valid/test-4.json.gz",
	}
	file_data := [7]string{
		"test-1.txt",
		"test-2.rdf.gz",
		"test-3.json",
		"test-4.json.gz",
		"test-5.txt",
		"test-6.txt.gz",
		"test-7.go",
	}

	for i, data := range file_data {
		var filePath string

		if i <= 4 {
			filePath = filepath.Join(validTestFiles, data)
		} else {
			filePath = filepath.Join(invalidTestFiles, data)
		}
		f, err := os.Create(filePath)
		require.NoError(t, err)
		defer f.Close()
	}
	filesList := FindDataFiles("./test-files-valid", []string{".rdf", ".rdf.gz", ".json", ".json.gz"})
	require.Equal(t, expectedFilesArray, filesList)

	filesList = FindDataFiles(invalidTestFiles, []string{".rdf", ".rdf.gz", ".json", ".json.gz"})
	require.Equal(t, 0, len(filesList))

	filesList = FindDataFiles("", []string{".rdf", ".rdf.gz", ".json", ".json.gz"})
	require.Equal(t, 0, len(filesList))
}

func TestIsMissingOrEmptyDir(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := "test-files"
	require.NoError(t, os.MkdirAll(dir, os.ModePerm))
	defer deleteDirs(t, dir)
	testFilesDir := filepath.Join(filepath.Dir(thisFile), "test-files")
	file_data := [2]string{"test-1.txt", "test-2.txt"}
	for _, data := range file_data {
		filePath := filepath.Join(testFilesDir, data)
		f, err := os.Create(filePath)
		require.NoError(t, err)
		defer f.Close()
	}
	//checking function with file which exist
	output := IsMissingOrEmptyDir("./test-files/test-1.txt")
	require.Equal(t, nil, output)
	//checking function with file which does not exist
	output = IsMissingOrEmptyDir("./test-files/doesnotexist.txt")
	require.NotEqual(t, nil, output)
	//checking function with directory which exist
	output = IsMissingOrEmptyDir("./test-files")
	require.Equal(t, nil, output)
	//checking function with directory which does not exist
	output = IsMissingOrEmptyDir("./doesnotexist")
	require.NotEqual(t, nil, output)
}

func deleteDirs(t *testing.T, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		fmt.Printf("Error removing direcotory: %s", err.Error())
	}
}
