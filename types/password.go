
package types

import (
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/dgraph-io/dgraph/x"
)

const (
	pwdLenLimit = 6
)

func GenerateFromPassword(password string) (string, error) {
	var result string
	var byt []byte
	var err error
	if len(password) < pwdLenLimit {
		return result, x.Errorf("Password too short, i.e. should has at least 6 chars")
	}
	// no need to generate salt outselves since salt is incorporated in the result as well
	// check wiki https://en.wikipedia.org/wiki/Bcrypt
	byt, err = bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return result, err
	}
	// follow the format like linux used: $2a$cost$crypted
	// get rid of '$', since '$' is the delimiter
	result = base64.StdEncoding.EncodeToString(byt)
	result = fmt.Sprintf("$2a$%d$%s", bcrypt.DefaultCost, result)

	return result, nil
}

func VerifyPassword(password, crypted string) error {
	if len(password) < pwdLenLimit || len(crypted) == 0 {
		return x.Errorf("invalid password/crypted string")
	}
	// password stored format like: $2a$10$crypted
	arr := strings.Split(strings.Trim(crypted, "$"), "$")
	x.AssertTruef(len(arr) == 3, "Password is corrupted")
	target := arr[2]
	
	byt, err := base64.StdEncoding.DecodeString(target)
	x.AssertTruef(err == nil, "Password is corrupted")
	
	return bcrypt.CompareHashAndPassword(byt, []byte(password))
}

