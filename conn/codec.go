/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package conn

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/dgraph-io/dgraph/x"
)

type Query struct {
	Data []byte
}

type Reply struct {
	Data []byte
	// TODO(manishrjain): Add an error here.
	// Error string
}

func writeHeader(rwc io.ReadWriteCloser, seq uint64,
	method string, data []byte) error {

	var bh bytes.Buffer
	var rerr error

	x.SetError(&rerr, binary.Write(&bh, binary.LittleEndian, seq))
	x.SetError(&rerr, binary.Write(&bh, binary.LittleEndian, int32(len(method))))
	x.SetError(&rerr, binary.Write(&bh, binary.LittleEndian, int32(len(data))))
	_, err := bh.Write([]byte(method))
	x.SetError(&rerr, err)
	if rerr != nil {
		return rerr
	}
	_, err = rwc.Write(bh.Bytes())
	return err
}

func parseHeader(rwc io.ReadWriteCloser, seq *uint64,
	method *string, plen *int32) error {

	var err error
	var sz int32
	x.SetError(&err, binary.Read(rwc, binary.LittleEndian, seq))
	x.SetError(&err, binary.Read(rwc, binary.LittleEndian, &sz))
	x.SetError(&err, binary.Read(rwc, binary.LittleEndian, plen))
	if err != nil {
		return err
	}

	buf := make([]byte, sz)
	n, err := rwc.Read(buf)
	if err != nil {
		return err
	}
	if n != int(sz) {
		return fmt.Errorf("Expected: %v. Got: %v\n", sz, n)
	}
	*method = string(buf)
	return nil
}
