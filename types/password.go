/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package types

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const (
	pwdLenLimit = 6
)

var (
	passShortErr   = errors.New("Password too short, i.e. should has at least 6 chars")
	invalidPassErr = errors.New("Invalid password/crypted string")
)

func Encrypt(plain string) (string, error) {
	if len(plain) < pwdLenLimit {
		return "", passShortErr
	}

	encrypted, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(encrypted), nil
}

func VerifyPassword(plain, encrypted string) error {
	if len(plain) < pwdLenLimit || len(encrypted) == 0 {
		return invalidPassErr
	}

	return bcrypt.CompareHashAndPassword([]byte(encrypted), []byte(plain))
}
