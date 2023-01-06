package x

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dir string

func TestWalkPathFunc_Positive(t *testing.T) {

	xDir, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}

	filesGot := WalkPathFunc(xDir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".go")
	})
	assert.True(t, len(filesGot) > 0, "Path doesn't exist: %s", xDir)
}

func TestWalkPathFunc_Negative(t *testing.T) {

	xDir, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}
	dir = xDir + "/x"

	filesGot := WalkPathFunc(dir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".go")
	})

	assert.True(t, len(filesGot) == 0, "This is a negative test case, please pass any wrong dir")
}

func TestWriteGroupIdFile(t *testing.T) {

	for i := 0; i < 3; i++ {
		dir, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
		}
		dir = filepath.Join(dir+"/For_WriteGroupIdFile", strconv.Itoa(i))
		os.MkdirAll(dir, 0700)

		switch i {
		case 0:
			got := WriteGroupIdFile(dir, uint32(i))
			assert.Equal(t, BadGroupID, got.Error(), "If group_id passed to a WriteGroupIdFile(), It returns 'ID written to group_id file must be a positive number' as an error")

		default:
			got := WriteGroupIdFile(dir, uint32(i))

			assert.True(t, got == nil, "Something Wrong")

		}
	}

	//Delete dirs
	defer deleteDirs("./For_WriteGroupIdFile")

}

func TestReadGroupIdFile(t *testing.T) {

	for i := 0; i < 3; i++ {

		xDir, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
		}
		dir = filepath.Join(xDir+"/For_WriteGroupIdFile", strconv.Itoa(i))
		os.MkdirAll(dir, 0700)
		groupFile := filepath.Join(dir, GroupIdFileName)
		f, _ := os.OpenFile(groupFile, os.O_CREATE|os.O_WRONLY, 0600)
		f.WriteString(strconv.Itoa(int(i)))
		f.WriteString("\n")
		f.Close()

	}

	for i := 0; i < 3; i++ {
		xDir, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
		}
		switch i {
		case 0:
			dir = filepath.Join("./ABC")
			group_id, err := ReadGroupIdFile(dir)

			assert := assert.New(t)
			assert.Equal(uint32(0), group_id, "If passed path is not a directory, ReadGroupIdFile() returns 0")
			assert.Nil(err, "If passed path is not a directory, ReadGroupIdFile() returns nil as an error")

		case 1:
			expectedErr := "Group ID file at /home/siddesh/workspace/dgraph_unit/x/For_WriteGroupIdFile/1/p/group_id is a directory"
			os.MkdirAll(xDir+"/For_WriteGroupIdFile/1/p/group_id", os.ModePerm)
			dir = filepath.Join(xDir+"/For_WriteGroupIdFile", strconv.Itoa(i), "p")
			group_id, err := ReadGroupIdFile(dir)

			assert := assert.New(t)
			assert.Equal(uint32(0), group_id, "If passed path is directory, ReadGroupIdFile() returns 0")
			assert.Equal(expectedErr, err.Error(), "If passed path is directory, ReadGroupIdFile() returns 'Group ID file at <dir> is a directory' as an error")

		case 2:
			dir = filepath.Join(xDir+"/For_WriteGroupIdFile", strconv.Itoa(i))
			group_id, err := ReadGroupIdFile(dir)

			assert := assert.New(t)
			assert.Equal(uint32(2), group_id, "If passed path is a group_id file, ReadGroupIdFile() returns <group_id>")
			assert.Nil(err, "If passed path is directory, ReadGroupIdFile() returns nil as an error")
		}
	}

	//Delete dirs
	defer deleteDirs("./For_WriteGroupIdFile")
}

func deleteDirs(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		fmt.Println(err)
	}
}

func TestFindDataFiles(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := [2]string{"test-files-valid", "test-files-invalid"}
	require.NoError(t, os.MkdirAll(dir[0], os.ModePerm))
	require.NoError(t, os.MkdirAll(dir[1], os.ModePerm))

	validTestFiles := filepath.Join(filepath.Dir(thisFile), "test-files-valid")
	invalidTestFiles := filepath.Join(filepath.Dir(thisFile), "test-files-invalid")
	expectedFilesArray := []string{"test-files-valid/test-2.rdf.gz", "test-files-valid/test-3.json", "test-files-valid/test-4.json.gz"}

	file_data := [7]string{"test-1.txt", "test-2.rdf.gz", "test-3.json", "test-4.json.gz", "test-5.txt", "test-6.txt.gz", "test-7.go"}

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

	if err := os.RemoveAll(validTestFiles); err != nil {
		t.Fatalf("Error removing direcotory: %s", err.Error())
	}
	if err := os.RemoveAll(invalidTestFiles); err != nil {
		t.Fatalf("Error removing direcotory: %s", err.Error())
	}

}

func TestIsMissingOrEmptyDir(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := "test-files"
	require.NoError(t, os.MkdirAll(dir, os.ModePerm))
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

	if err := os.RemoveAll(testFilesDir); err != nil {
		t.Fatalf("Error removing direcotory: %s", err.Error())
	}
}
