package graphql

import (
	"fmt"
	"io"
)

type Upload struct {
	File     io.Reader
	Filename string
	Size     int64
}

func MarshalUpload(f Upload) Marshaler {
	return WriterFunc(func(w io.Writer) {
		io.Copy(w, f.File)
	})
}

func UnmarshalUpload(v interface{}) (Upload, error) {
	upload, ok := v.(Upload)
	if !ok {
		return Upload{}, fmt.Errorf("%T is not an Upload", v)
	}
	return upload, nil
}
