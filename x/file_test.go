package x

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var dir string

func TestWalkPathFunc_Positive(t *testing.T) {

	dir := "../x"

	filesGot := WalkPathFunc(dir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".go")
	})

	if len(filesGot) == 0 {
		t.Errorf("Path doesn't exist: %s", dir)
	}
}

func TestWalkPathFunc_Negative(t *testing.T) {

	dir := "./x"

	filesGot := WalkPathFunc(dir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".go")
	})

	if len(filesGot) != 0 {
		t.Errorf("Path exist: %s, for this test try passing wrong path", dir)
	}
}

func TestWriteGroupIdFile(t *testing.T) {

	for i := 0; i < 3; i++ {
		dir := filepath.Join("./For_WriteGroupIdFile", strconv.Itoa(i))
		os.MkdirAll(dir, 0700)

		switch i {
		case 0:
			got := WriteGroupIdFile(dir, uint32(i)).Error()
			want := "ID written to group_id file must be a positive number"

			if got != want {
				t.Errorf("got : %s, wanted : %s", got, want)
			}

		case 1:
			got := WriteGroupIdFile(dir, uint32(i))

			if got != nil {
				t.Errorf("got : %s, wanted : nil", got)
			}

		case 2:

			WriteGroupIdFile(dir, uint32(i))

		}
	}

	//Delete dirs
	deleteDirs("./For_WriteGroupIdFile")

}

func TestReadGroupIdFile(t *testing.T) {

	for i := 0; i < 3; i++ {

		dir := filepath.Join("./For_WriteGroupIdFile", strconv.Itoa(i))
		os.MkdirAll(dir, 0700)
		groupFile := filepath.Join(dir, GroupIdFileName)
		f, _ := os.OpenFile(groupFile, os.O_CREATE|os.O_WRONLY, 0600)
		f.WriteString(strconv.Itoa(int(i)))
		f.WriteString("\n")
		f.Close()

	}

	for i := 0; i < 3; i++ {
		switch i {
		case 0:
			dir = filepath.Join("./ABC")
			group_id, err := ReadGroupIdFile(dir)

			if group_id != 0 && err != nil {
				t.Errorf("Something Wrong !")
			}
		case 1:
			os.MkdirAll("./For_WriteGroupIdFile/1/p/group_id", os.ModePerm)
			dir = filepath.Join("./For_WriteGroupIdFile", strconv.Itoa(i), "p")
			group_id, err := ReadGroupIdFile(dir)

			if group_id != 0 || err == nil {
				t.Errorf("got: dir, wanted: path")
			}
		case 2:
			dir = filepath.Join("./For_WriteGroupIdFile", strconv.Itoa(2))
			group_id, err := ReadGroupIdFile(dir)

			if group_id != 2 && err == nil {
				t.Errorf("got: %v , Wanted: 2", group_id)
			}
		}
	}

	//Delete dirs
	deleteDirs("./For_WriteGroupIdFile")
}

func deleteDirs(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		fmt.Println(err)
	}
}
