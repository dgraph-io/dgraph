/*
 * Copyright 2016 Dgraph Labs, Inc.
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

// Package icuembed contains ICU common lib. It is for embedding into our binary
// so that the user does not have to install ICU.
package icuembed

// #cgo CPPFLAGS: -O2 -DU_COMMON_IMPLEMENTATION  -DU_DISABLE_RENAMING=1 -Wno-deprecated-declarations
// #cgo LDFLAGS: -ldl
// #cgo darwin LDFLAGS: -Wl,-undefined -Wl,dynamic_lookup
// #cgo !darwin LDFLAGS: -Wl,-unresolved-symbols=ignore-all -lrt
// #include <unicode/utypes.h>
// #include <unicode/udata.h>
import "C"

import (
	"fmt"
	"io/ioutil"
	"log"
	"time"
	"unsafe"
)

var icuData []byte // Hold a reference.

// Load loads ICU data from input filename, and call
// udata_setCommonData to initialize ICU. Without this, ICU will not work.
func Load(filename string) error {
	if len(filename) == 0 {
		return fmt.Errorf("Invalid data file")
	}

	start := time.Now()
	icuData, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	var icuErr C.UErrorCode
	C.udata_setCommonData(unsafe.Pointer(byteToChar(icuData)), &icuErr)
	if int(icuErr) < 0 {
		return fmt.Errorf("udata_setCommonData failed: %d", int(icuErr))
	}

	log.Printf("Loaded ICU data from [%s] in %s", filename, time.Since(start))
	return nil
}

// byteToChar returns *C.char from byte slice.
func byteToChar(b []byte) *C.char {
	var c *C.char
	if len(b) > 0 {
		c = (*C.char)(unsafe.Pointer(&b[0]))
	}
	return c
}
