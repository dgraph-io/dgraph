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
	"context"
	"encoding/json"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
	"net/http"
	"strings"
	"time"
)

type ctxKey string

const (
	AuthJwtCtxKey = ctxKey("authorizationJwt")
)

var (
	metainfo = &AuthMeta{}
)

type AuthMeta struct {
	PublicKey string
	Header    string
	Namespace string
}

func (m *AuthMeta) Parse(schema string) {
	poundIdx := strings.LastIndex(schema, "#")
	if poundIdx == -1 {
		return
	}
	lastComment := schema[poundIdx:]
	length := strings.Index(lastComment, "\n")
	if length > -1 {
		lastComment = lastComment[:length]
	}

	authMeta := strings.Split(lastComment, " ")
	if len(authMeta) != 5 || authMeta[1] != "Authorization" {
		return
	}
	m.Header = authMeta[2]
	m.PublicKey = authMeta[3]
	m.Namespace = authMeta[4]
}

func ParseAuthMeta(schema string) {
	metainfo.Parse(schema)
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
	if metainfo.PublicKey == "" {
		return nil, fmt.Errorf(" jwt token cannot be validated because public key is empty")
	}

	token, err :=
		jwt.ParseWithClaims(jwtStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(metainfo.PublicKey), nil
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
