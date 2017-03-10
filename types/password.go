package types

import (
	"golang.org/x/crypto/bcrypt"

	"github.com/dgraph-io/dgraph/x"
)

const (
	pwdLenLimit = 6
)

func Encrypt(plain string) (string, error) {
	if len(plain) < pwdLenLimit {
		return "", x.Errorf("Password too short, i.e. should has at least 6 chars")
	}

	encrypted, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
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
