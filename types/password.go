
package types

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/dgraph-io/dgraph/x"
)

const (
	saltLen = 32
	pwdLenLimit = 6
)

func GenerateFromPassword(password string) (string, error) {
	var salt, result string
	var byt []byte
	var err error

	if len(password) < pwdLenLimit {
		return result, x.Errorf("Password too short, i.e. should has at least 6 chars")
	}

	byt = make([]byte, saltLen)
	if _, err = io.ReadFull(rand.Reader, byt); err != nil {
		return result, err
	}
	salt = base64.StdEncoding.EncodeToString(byt)

	byt, err = bcrypt.GenerateFromPassword([]byte(compose(salt, password)), bcrypt.DefaultCost)
	if err != nil {
		return result, err
	}
	// follow the format like linux used: $2b$cost$salt$crypted
	// get rid of '$', since '$' is the delimiter
	result = base64.StdEncoding.EncodeToString(byt)
	result = fmt.Sprintf("$2b$%d$%s$%s", bcrypt.DefaultCost, salt, result)

	return result, nil
}

func VerifyPassword(password, crypted string) error {
	if len(password) < pwdLenLimit || len(crypted) == 0 {
		return x.Errorf("invalid password/crypted string")
	}
	// password stored format like: $2b$10$salt$cryptedstr
	arr := strings.Split(strings.Trim(crypted, "$"), "$")
	x.AssertTruef(len(arr) == 4, "Password is corrupted")
	salt, target := arr[2], arr[3]
	
	byt, err := base64.StdEncoding.DecodeString(target)
	x.AssertTruef(err == nil, "Password is corrupted")
	
	return bcrypt.CompareHashAndPassword(byt, []byte(compose(salt, password)))
}

func compose(salt, password string) string {
	return salt + password
}

