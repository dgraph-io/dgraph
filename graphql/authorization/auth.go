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
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

type ctxKey string

const (
	AuthJwtCtxKey = ctxKey("authorizationJwt")
	RSA256        = "RS256"
	HMAC256       = "HS256"
)

var (
	metainfo = &AuthMeta{}
)

type AuthMeta struct {
	HMACPublicKey string
	RSAPublicKey  *rsa.PublicKey
	Header        string
	Namespace     string
	Algo          string
}

func (m *AuthMeta) Parse(schema string) error {
	lastCommentIdx := strings.LastIndex(schema, "#")
	if lastCommentIdx == -1 {
		return nil
	}
	lastComment := schema[lastCommentIdx:]
	if !strings.HasPrefix(lastComment, "# Authorization") {
		return nil
	}

	// This regex matches authorization information present in the last line of the schema.
	// Format: # Authorization <HTTP header> <Claim namespace> <Algorithm> "<verification key>"
	// Example: # Authorization X-Test-Auth https://xyz.io/jwt/claims HS256 "secretkey"
	// On successful regex match the index for the following strings will be returned.
	// [0][0]:[0][1] : # Authorization X-Test-Auth https://xyz.io/jwt/claims HS256 "secretkey"
	// [0][2]:[0][3] : Authorization, [0][4]:[0][5] : X-Test-Auth,
	// [0][6]:[0][7] : https://xyz.io/jwt/claims,
	// [0][8]:[0][9] : HS256, [0][10]:[0][11] : secretkey
	authMetaRegex, err :=
		regexp.Compile(`^#[\s]([^\s]+)[\s]+([^\s]+)[\s]+([^\s]+)[\s]+([^\s]+)[\s]+"([^\"]+)"`)
	if err != nil {
		return errors.Errorf("error while parsing jwt authorization info: %v", err)
	}
	idx := authMetaRegex.FindAllStringSubmatchIndex(lastComment, -1)
	if len(idx) != 1 || len(idx[0]) != 12 ||
		!strings.HasPrefix(lastComment, lastComment[idx[0][0]:idx[0][1]]) {
		return errors.Errorf("error while parsing jwt authorization info")
	}

	m.Header = lastComment[idx[0][4]:idx[0][5]]
	m.Namespace = lastComment[idx[0][6]:idx[0][7]]
	m.Algo = lastComment[idx[0][8]:idx[0][9]]

	key := lastComment[idx[0][10]:idx[0][11]]
	if m.Algo == HMAC256 {
		m.HMACPublicKey = key
		return nil
	}
	if m.Algo != RSA256 {
		return errors.Errorf(
			"invalid jwt algorithm: found %s, but supported options are HS256 or RS256", m.Algo)
	}

	// The jwt library internally uses `bytes.IndexByte(data, '\n')` to fetch new line and fails
	// if we have newline "\n" as ASCII value {92,110} instead of the actual ASCII value of 10.
	// To fix this we replace "\n" with new line's ASCII value.
	bytekey := bytes.ReplaceAll([]byte(key), []byte{92, 110}, []byte{10})

	m.RSAPublicKey, err = jwt.ParseRSAPublicKeyFromPEM(bytekey)
	return err
}

func ParseAuthMeta(schema string) error {
	return metainfo.Parse(schema)
}

// AttachAuthorizationJwt adds any incoming JWT authorization data into the grpc context metadata.
func AttachAuthorizationJwt(ctx context.Context, r *http.Request) context.Context {
	authorizationJwt := r.Header.Get(metainfo.Header)
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
	if authVariables, ok := result[metainfo.Namespace]; ok {
		c.AuthVariables, _ = authVariables.(map[string]interface{})
	}
	return nil
}

func ExtractAuthVariables(ctx context.Context) (map[string]interface{}, error) {
	// Extract the jwt and unmarshal the jwt to get the auth variables.
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, nil
	}

	jwtToken := md.Get(string(AuthJwtCtxKey))
	if len(jwtToken) == 0 {
		return nil, nil
	} else if len(jwtToken) > 1 {
		return nil, fmt.Errorf("invalid jwt auth token")
	}

	return validateToken(jwtToken[0])
}

func validateToken(jwtStr string) (map[string]interface{}, error) {
	if metainfo.Algo == "" {
		return nil, fmt.Errorf(
			"jwt token cannot be validated because verification algorithm is not set")
	}

	token, err :=
		jwt.ParseWithClaims(jwtStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			algo, _ := token.Header["alg"].(string)
			if algo != metainfo.Algo {
				return nil, errors.Errorf("unexpected signing method: Expected %s Found %s",
					metainfo.Algo, algo)
			}
			if algo == HMAC256 {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
					return []byte(metainfo.HMACPublicKey), nil
				}
			} else if algo == RSA256 {
				if _, ok := token.Method.(*jwt.SigningMethodRSA); ok {
					return metainfo.RSAPublicKey, nil
				}
			}
			return nil, errors.Errorf("couldn't parse signing method from token header: %s", algo)
		})

	if err != nil {
		return nil, errors.Errorf("unable to parse jwt token:%v", err)
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, errors.Errorf("claims in jwt token is not map claims")
	}

	// by default, the MapClaims.Valid will return true if the exp field is not set
	// here we enforce the checking to make sure that the refresh token has not expired
	now := time.Now().Unix()
	if !claims.VerifyExpiresAt(now, true) {
		return nil, errors.Errorf("Token is expired") // the same error msg that's used inside jwt-go
	}

	return claims.AuthVariables, nil
}
