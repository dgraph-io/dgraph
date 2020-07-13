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
	"strings"

	"github.com/dgrijalva/jwt-go/v4"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

type ctxKey string

const (
	AuthJwtCtxKey  = ctxKey("authorizationJwt")
	RSA256         = "RS256"
	HMAC256        = "HS256"
	AuthMetaHeader = "# Dgraph.Authorization "
)

var (
	metainfo AuthMeta
)

type AuthMeta struct {
	VerificationKey string
	RSAPublicKey    *rsa.PublicKey `json:"-"` // Ignoring this field
	Header          string
	Namespace       string
	Algo            string
	Audience        []string
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

func Parse(schema string) (AuthMeta, error) {
	var meta AuthMeta
	authInfoIdx := strings.LastIndex(schema, AuthMetaHeader)
	if authInfoIdx == -1 {
		return meta, nil
	}
	authInfo := schema[authInfoIdx:]

	err := json.Unmarshal([]byte(authInfo[len(AuthMetaHeader):]), &meta)
	if err != nil {
		return meta, fmt.Errorf("Unable to parse Dgraph.Authorization. " +
			"It may be that you are using the pre-release syntax. " +
			"Please check the correct syntax at https://graphql.dgraph.io/authorization/")
	}
	return meta, meta.validate()
}

func ParseAuthMeta(schema string) error {
	var err error
	metainfo, err = Parse(schema)
	if err != nil {
		return err
	}

	if metainfo.Algo != RSA256 {
		return err
	}

	// The jwt library internally uses `bytes.IndexByte(data, '\n')` to fetch new line and fails
	// if we have newline "\n" as ASCII value {92,110} instead of the actual ASCII value of 10.
	// To fix this we replace "\n" with new line's ASCII value.
	bytekey := bytes.ReplaceAll([]byte(metainfo.VerificationKey), []byte{92, 110}, []byte{10})

	metainfo.RSAPublicKey, err = jwt.ParseRSAPublicKeyFromPEM(bytekey)
	return err
}

func GetHeader() string {
	return metainfo.Header
}

func GetAuthMeta() AuthMeta {
	return metainfo
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
	if authValue, ok := result[metainfo.Namespace]; ok {
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

func (c *CustomClaims) validateAudience() error {
	// If there's no audience claim, ignore
	if c.Audience == nil || len(c.Audience) == 0 {
		return nil
	}

	// If there is an audience claim, but no value provided, fail
	if metainfo.Audience == nil {
		return fmt.Errorf("audience value was expected but not provided")
	}

	var match = false
	for _, audStr := range c.Audience {
		for _, expectedAudStr := range metainfo.Audience {
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

func validateToken(jwtStr string) (map[string]interface{}, error) {
	if metainfo.Algo == "" {
		return nil, fmt.Errorf(
			"jwt token cannot be validated because verification algorithm is not set")
	}

	// The JWT library supports comparison of `aud` in JWT against a single string. Hence, we
	// disable the `aud` claim verification at the library end using `WithoutClaimsValidation` and
	// use our custom validation function `validateAudience`.
	token, err :=
		jwt.ParseWithClaims(jwtStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			algo, _ := token.Header["alg"].(string)
			if algo != metainfo.Algo {
				return nil, errors.Errorf("unexpected signing method: Expected %s Found %s",
					metainfo.Algo, algo)
			}
			if algo == HMAC256 {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
					return []byte(metainfo.VerificationKey), nil
				}
			} else if algo == RSA256 {
				if _, ok := token.Method.(*jwt.SigningMethodRSA); ok {
					return metainfo.RSAPublicKey, nil
				}
			}
			return nil, errors.Errorf("couldn't parse signing method from token header: %s", algo)
		}, jwt.WithoutClaimsValidation())

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

	return claims.AuthVariables, nil
}
