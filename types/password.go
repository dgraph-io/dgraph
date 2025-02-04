/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import (
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
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
