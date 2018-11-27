/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

import (
	"strconv"

	"golang.org/x/crypto/bcrypt"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

const (
	pwdLenLimit = 6
)

func Encrypt(plain string) (string, error) {
	if len(plain) < pwdLenLimit {
		return "", x.Errorf("Password too short, i.e. should has at least 6 chars")
	}

	b := []byte(plain)

	// maybe already encrypted, most likely live import.
	if isBcryptHash(b) {
		glog.V(3).Infof("Encrypt password already encrypted, using it.")
		return plain, nil
	}

	encrypted, err := bcrypt.GenerateFromPassword(b, bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(encrypted), nil
}

func VerifyPassword(plain, encrypted string) error {
	if len(plain) < pwdLenLimit || len(encrypted) == 0 {
		return x.Errorf("invalid password/crypted string")
	}

	return bcrypt.CompareHashAndPassword([]byte(encrypted), []byte(plain))
}

const bcryptMinHashSize = 59

// isBcryptHash checks if the slice es resembles a bcrypt hash.
// Returns true if es looks like a bcrypt hash, false otherwise.
func isBcryptHash(es []byte) bool {
	if len(es) < bcryptMinHashSize {
		return false
	}
	if es[0] != '$' {
		return false
	}
	if es[1] != '2' {
		return false
	}
	n := 3
	if es[2] != '$' {
		n++ // skip over minor
	}
	cost, err := strconv.Atoi(string(es[n : n+2]))
	if err != nil {
		return false
	}
	return cost >= bcrypt.MinCost && cost <= bcrypt.MaxCost
}
