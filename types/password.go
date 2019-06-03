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
	"golang.org/x/crypto/bcrypt"

	"github.com/pkg/errors"
)

const (
	pwdLenLimit = 6
)

// Encrypt encrypts the given plain-text password.
func Encrypt(plain string) (string, error) {
	if len(plain) < pwdLenLimit {
		return "", errors.Errorf("Password too short, i.e. should have at least 6 chars")
	}

	encrypted, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(encrypted), nil
}

// VerifyPassword checks that the plain-text password matches the encrypted password.
func VerifyPassword(plain, encrypted string) error {
	if len(plain) < pwdLenLimit || len(encrypted) == 0 {
		return errors.Errorf("Invalid password/crypted string")
	}

	return bcrypt.CompareHashAndPassword([]byte(encrypted), []byte(plain))
}
