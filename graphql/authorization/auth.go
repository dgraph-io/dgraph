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
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
	"net/http"
	"time"
)

const (
	AuthJwtCtxKey = "authorizationJwt"
)

var (
	AuthHmacSecret string
	AuthHeader     string
)

// AttachAuthorizationJwt adds any incoming JWT authorization data into the grpc context metadata.
func AttachAuthorizationJwt(ctx context.Context, r *http.Request) context.Context {
	authorizationJwt := r.Header.Get(AuthHeader)
	if authorizationJwt == "" {
		return ctx
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}

	md.Append(AuthJwtCtxKey, authorizationJwt)
	ctx = metadata.NewIncomingContext(ctx, md)
	return ctx
}

type CustomClaims struct {
	AuthVariables map[string]interface{} `json:"https://dgraph.io/jwt/claims"`
	jwt.StandardClaims
}

func ExtractAuthVariables(ctx context.Context) (map[string]interface{}, error) {
	// Extract the jwt and unmarshal the jwt to get the auth variables.
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, nil
	}

	jwtToken := md.Get(AuthJwtCtxKey)
	if len(jwtToken) == 0 {
		return nil, nil
	} else if len(jwtToken) > 1 {
		return nil, fmt.Errorf("invalid jwt auth token")
	}

	return validateToken(jwtToken[0])
}

func validateToken(jwtStr string) (map[string]interface{}, error) {
	if AuthHmacSecret == "" {
		return nil, fmt.Errorf(" jwt token cannot be validated because secret key is empty")
	}
	token, err :=
		jwt.ParseWithClaims(jwtStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(AuthHmacSecret), nil
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
