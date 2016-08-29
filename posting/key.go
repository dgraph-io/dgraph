/*
 * Copyright 2015 DGraph Labs, Inc.
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

package posting

import (
	"bytes"
	"encoding/binary"
	"log"
)

// Key serializes both UID and attribute into a []byte key.
func Key(uid uint64, attr string) []byte {
	buf := bytes.NewBufferString(attr)
	buf.WriteRune('|')
	if err := binary.Write(buf, binary.LittleEndian, uid); err != nil {
		log.Fatalf("Error while creating key with attr: %v uid: %v\n", attr, uid)
	}
	return buf.Bytes()
}

// UID returns UID encoded as []byte.
func UID(uid uint64) []byte {
	out := make([]byte, 8)
	binary.LittleEndian.PutUint64(out, uid)
	return out
}

// DecodeKey deserializes the uid and attr.
func DecodeKey(b []byte) (uint64, string) {
	buf := bytes.NewBuffer(b)
	attr, err := buf.ReadString('|')
	if err != nil {
		log.Fatalf("Error while decoding key: %v", b)
	}
	attr = attr[:len(attr)-1]
	var uid uint64
	err = binary.Read(buf, binary.LittleEndian, &uid)
	if err != nil {
		log.Fatalf("Error while decoding key: %v", b)
	}
	return uid, attr
}

// DecodeKeyPartial deserializes the attr and returns uid as serialized.
func DecodeKeyPartial(b []byte) ([]byte, string) {
	buf := bytes.NewBuffer(b)
	attr, err := buf.ReadString('|')
	if err != nil {
		log.Fatalf("Error while decoding key: %v", b)
	}
	uid := b[len(attr):]
	attr = attr[:len(attr)-1]
	return uid, attr
}

// DecodeUID deserializes []byte into uint64.
func DecodeUID(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}
