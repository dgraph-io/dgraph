/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package authorization

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/dgrijalva/jwt-go/v4"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

type ctxKey string
type authVariablekey string

const (
	AuthJwtCtxKey  = ctxKey("authorizationJwt")
	AuthVariables  = authVariablekey("authVariable")
	RSA256         = "RS256"
	HMAC256        = "HS256"
	AuthMetaHeader = "# Dgraph.Authorization "
)

var (
	authMeta = &AuthMeta{}
)

type AuthMeta struct {
	VerificationKey string
	RSAPublicKey    *rsa.PublicKey `json:"-"` // Ignoring this field
	Header          string
	Namespace       string
	Algo            string
	Audience        []string
	sync.RWMutex
}

// Validate required fields.
func (a *AuthMeta) validate() error {
	var fields string
	if a.VerificationKey == "" {
		fields = " `Verification key`"
	}

	if a.Header == "" {
		fields += " `Header`"
	}

	if a.Namespace == "" {
		fields += " `Namespace`"
	}

	if a.Algo == "" {
		fields += " `Algo`"
	}

	if len(fields) > 0 {
		return fmt.Errorf("required field missing in Dgraph.Authorization:%s", fields)
	}
	return nil
}

func Parse(schema string) (*AuthMeta, error) {
	var meta AuthMeta
	authInfoIdx := strings.LastIndex(schema, AuthMetaHeader)
	if authInfoIdx == -1 {
		return nil, nil
	}
	authInfo := schema[authInfoIdx:]

	err := json.Unmarshal([]byte(authInfo[len(AuthMetaHeader):]), &meta)
	if err == nil {
		return &meta, meta.validate()
	}

	fmt.Println("Falling back to parsing `Dgraph.Authorization` in old format." +
		" Please check the updated syntax at https://graphql.dgraph.io/authorization/")
	// Note: This is the old format for passing authorization information and this code
	// is there to maintain backward compatibility. It may be removed in future release.

	// This regex matches authorization information present in the last line of the schema.
	// Format: # Dgraph.Authorization <HTTP header> <Claim namespace> <Algorithm> "<verification key>"
	// Example: # Dgraph.Authorization X-Test-Auth https://xyz.io/jwt/claims HS256 "secretkey"
	// On successful regex match the index for the following strings will be returned.
	// [0][0]:[0][1] : # Dgraph.Authorization X-Test-Auth https://xyz.io/jwt/claims HS256 "secretkey"
	// [0][2]:[0][3] : Authorization, [0][4]:[0][5] : X-Test-Auth,
	// [0][6]:[0][7] : https://xyz.io/jwt/claims,
	// [0][8]:[0][9] : HS256, [0][10]:[0][11] : secretkey
	authMetaRegex, err :=
		regexp.Compile(`^#[\s]([^\s]+)[\s]+([^\s]+)[\s]+([^\s]+)[\s]+([^\s]+)[\s]+"([^\"]+)"`)
	if err != nil {
		return nil, gqlerror.Errorf("JWT parsing failed: %v", err)
	}

	idx := authMetaRegex.FindAllStringSubmatchIndex(authInfo, -1)
	if len(idx) != 1 || len(idx[0]) != 12 ||
		!strings.HasPrefix(authInfo, authInfo[idx[0][0]:idx[0][1]]) {
		return nil, gqlerror.Errorf("Invalid `Dgraph.Authorization` format: %s", authInfo)
	}

	meta.Header = authInfo[idx[0][4]:idx[0][5]]
	meta.Namespace = authInfo[idx[0][6]:idx[0][7]]
	meta.Algo = authInfo[idx[0][8]:idx[0][9]]
	meta.VerificationKey = authInfo[idx[0][10]:idx[0][11]]
	if meta.Algo == HMAC256 {
		return &meta, nil
	}
	if meta.Algo != RSA256 {
		return nil, errors.Errorf(
			"invalid jwt algorithm: found %s, but supported options are HS256 or RS256", meta.Algo)
	}
	return &meta, nil
}

func ParseAuthMeta(schema string) (*AuthMeta, error) {
	metaInfo, err := Parse(schema)
	if err != nil {
		return nil, err
	}

	if metaInfo.Algo != RSA256 {
		return metaInfo, nil
	}

	// The jwt library internally uses `bytes.IndexByte(data, '\n')` to fetch new line and fails
	// if we have newline "\n" as ASCII value {92,110} instead of the actual ASCII value of 10.
	// To fix this we replace "\n" with new line's ASCII value.
	bytekey := bytes.ReplaceAll([]byte(metaInfo.VerificationKey), []byte{92, 110}, []byte{10})

	if metaInfo.RSAPublicKey, err = jwt.ParseRSAPublicKeyFromPEM(bytekey); err != nil {
		return nil, err
	}
	return metaInfo, nil
}

func GetHeader() string {
	authMeta.RLock()
	defer authMeta.RUnlock()
	return authMeta.Header
}

func GetAuthMeta() *AuthMeta {
	authMeta.RLock()
	defer authMeta.RUnlock()
	return authMeta
}

func SetAuthMeta(m *AuthMeta) {
	authMeta.Lock()
	defer authMeta.Unlock()

	authMeta.VerificationKey = m.VerificationKey
	authMeta.RSAPublicKey = m.RSAPublicKey
	authMeta.Header = m.Header
	authMeta.Namespace = m.Namespace
	authMeta.Algo = m.Algo
	authMeta.Audience = m.Audience
}

// AttachAuthorizationJwt adds any incoming JWT authorization data into the grpc context metadata.
func AttachAuthorizationJwt(ctx context.Context, r *http.Request) context.Context {
	authorizationJwt := r.Header.Get(authMeta.Header)
	if authorizationJwt == "" {
		return ctx
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}

	md.Append(string(AuthJwtCtxKey), authorizationJwt)
	ctx = metadata.NewIncomingContext(ctx, md)
	return ctx
}

type CustomClaims struct {
	AuthVariables map[string]interface{}
	jwt.StandardClaims
}

func (c *CustomClaims) UnmarshalJSON(data []byte) error {
	// Unmarshal the standard claims first.
	if err := json.Unmarshal(data, &c.StandardClaims); err != nil {
		return err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}

	// Unmarshal the auth variables for a particular namespace.
	if authValue, ok := result[authMeta.Namespace]; ok {
		if authJson, ok := authValue.(string); ok {
			if err := json.Unmarshal([]byte(authJson), &c.AuthVariables); err != nil {
				return err
			}
		} else {
			c.AuthVariables, _ = authValue.(map[string]interface{})
		}
	}
	return nil
}

func (c *CustomClaims) validateAudience() error {
	// If there's no audience claim, ignore
	if c.Audience == nil || len(c.Audience) == 0 {
		return nil
	}

	// If there is an audience claim, but no value provided, fail
	if authMeta.Audience == nil {
		return fmt.Errorf("audience value was expected but not provided")
	}

	var match = false
	for _, audStr := range c.Audience {
		for _, expectedAudStr := range authMeta.Audience {
			if subtle.ConstantTimeCompare([]byte(audStr), []byte(expectedAudStr)) == 1 {
				match = true
				break
			}
		}
	}
	if !match {
		return fmt.Errorf("JWT `aud` value doesn't match with the audience")
	}
	return nil
}

func ExtractCustomClaims(ctx context.Context) (*CustomClaims, error) {
	// return CustomClaims containing jwt and authvariables.
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return &CustomClaims{}, nil
	}

	jwtToken := md.Get(string(AuthJwtCtxKey))
	if len(jwtToken) == 0 {
		return &CustomClaims{}, nil
	} else if len(jwtToken) > 1 {
		return nil, fmt.Errorf("invalid jwt auth token")
	}
	return validateJWTCustomClaims(jwtToken[0])
}

func validateJWTCustomClaims(jwtStr string) (*CustomClaims, error) {
	authMeta.RLock()
	defer authMeta.RUnlock()

	if authMeta.Algo == "" {
		return nil, fmt.Errorf(
			"jwt token cannot be validated because verification algorithm is not set")
	}

	// The JWT library supports comparison of `aud` in JWT against a single string. Hence, we
	// disable the `aud` claim verification at the library end using `WithoutAudienceValidation` and
	// use our custom validation function `validateAudience`.
	token, err :=
		jwt.ParseWithClaims(jwtStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			algo, _ := token.Header["alg"].(string)
			if algo != authMeta.Algo {
				return nil, errors.Errorf("unexpected signing method: Expected %s Found %s",
					authMeta.Algo, algo)
			}
			if algo == HMAC256 {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
					return []byte(authMeta.VerificationKey), nil
				}
			} else if algo == RSA256 {
				if _, ok := token.Method.(*jwt.SigningMethodRSA); ok {
					return authMeta.RSAPublicKey, nil
				}
			}
			return nil, errors.Errorf("couldn't parse signing method from token header: %s", algo)
		}, jwt.WithoutAudienceValidation())

	if err != nil {
		return nil, errors.Errorf("unable to parse jwt token:%v", err)
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, errors.Errorf("claims in jwt token is not map claims")
	}

	if err := claims.validateAudience(); err != nil {
		return nil, err
	}
	return claims, nil
}
