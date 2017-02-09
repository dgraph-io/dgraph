
package types

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/bcrypt"

	"github.com/dgraph-io/dgraph/x"
)

const (
	PW_SALT_BYTES = 32
	//PW_HASH_BYTES = 64
	PW_LEN_LIMIT  = 6
)

func GenerateFromPassword(password string) (string, error) {
	var salt, result string
	var byt []byte
	var err error

	fmt.Printf("[GenerateFromPassword] entered\n")
	
	if len(password) < PW_LEN_LIMIT {
		return result, x.Errorf("Password too short, i.e. should has at least 6 chars")
	}

	byt = make([]byte, PW_SALT_BYTES)
	if _, err = io.ReadFull(rand.Reader, byt); err != nil {
		return result, err
	}
	salt = base64.StdEncoding.EncodeToString(byt)

	byt, err = bcrypt.GenerateFromPassword([]byte(salt + password), bcrypt.DefaultCost)
	if err != nil {
		return result, err
	}
	result = base64.StdEncoding.EncodeToString(byt)

	result = fmt.Sprintf("$2b$%d$%s$%s", bcrypt.DefaultCost, salt, result)

	fmt.Printf("result is %v\n", result)
	return result, nil
}
