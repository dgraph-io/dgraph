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

package x

import (
	"bytes"
	"encoding/binary"
)

// EncodeUint64 encodes value using little endian.
func EncodeUint64(v uint64) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
		return nil, Wrap(err)
	}
	return buf.Bytes(), nil
}

// DecodeUint64 decodes value in little endian.
func DecodeUint64(b []byte) (uint64, error) {
	buf := bytes.NewBuffer(b)
	var v uint64
	if err := binary.Read(buf, binary.LittleEndian, &v); err != nil {
		return 0, Wrap(err)
	}
	return v, nil
}

// EncodeUint64Ordered encodes value using big endian. String ordering will imply
// that the uint64s are sorted.
func EncodeUint64Ordered(v uint64) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, v); err != nil {
		return nil, Wrap(err)
	}
	return buf.Bytes(), nil
}

// DecodeUint64Ordered decodes value using big endian.
func DecodeUint64Ordered(b []byte) (uint64, error) {
	buf := bytes.NewBuffer(b)
	var v uint64
	if err := binary.Read(buf, binary.BigEndian, &v); err != nil {
		return 0, Wrap(err)
	}
	return v, nil
}
