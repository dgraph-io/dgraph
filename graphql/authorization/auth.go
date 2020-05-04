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
	poundIdx := strings.LastIndex(schema, "#")
	if poundIdx == -1 {
		return nil
	}
	lastComment := schema[poundIdx:]
	if !strings.HasPrefix(lastComment, "# Authorization") {
		return nil
	}

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
	if m.Algo == "HS256" {
		m.HMACPublicKey = key
		return nil
	}
	if m.Algo != "RS256" {
		return errors.Errorf("invalid jwt algorithm: %s", m.Algo)
	}

	// Replace "\n" with ASCII new line value.
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
	if len(metainfo.Algo) == 0 {
		return nil, fmt.Errorf(
			"jwt token cannot be validated because verificaiton algorithm is not set")
	}
	if metainfo.Algo == "HS256" && metainfo.HMACPublicKey == "" {
		return nil, fmt.Errorf("jwt token cannot be validated because HMAC key is empty")
	}
	if metainfo.Algo == "RS256" && metainfo.RSAPublicKey == nil {
		return nil, fmt.Errorf("jwt token cannot be validated because RSA public key is empty")
	}

	token, err :=
		jwt.ParseWithClaims(jwtStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			algo, _ := token.Header["alg"].(string)
			if algo != metainfo.Algo {
				return nil, errors.Errorf("unexpected signing method: Expected %s Found %s",
					metainfo.Algo, algo)
			}
			if algo == "HS256" {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
					return []byte(metainfo.HMACPublicKey), nil
				}
			} else if algo == "RS256" {
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
