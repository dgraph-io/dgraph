package bidx

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
)

// TODO: Rename as SchemaEntry
// Describes a single index.
type Entry struct {
	Type string // Index type: text, number, datetime, bool.
	Name string // Predicate / attribute.
}

type Schema []*Entry

func NewSchema(basedir string) (Schema, error) {
	f, err := ioutil.ReadFile(path.Join(basedir, "schema.json"))
	if err != nil {
		return nil, err
	}
	s := &Schema{}
	err = json.Unmarshal(f, s)
	if err != nil {
		return nil, err
	}
	return *s, nil
}

func (s Schema) Write(basedir string) error {
	f, err := os.Create(path.Join(basedir, "schema.json"))
	if err != nil {
		return err
	}
	defer f.Close()

	js, err := json.Marshal(s)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(f)
	defer w.Flush()

	_, err = w.Write(js)
	if err != nil {
		return err
	}
	return nil
}
