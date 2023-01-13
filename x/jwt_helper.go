/*
 * Copyright 2017-2023 Dgraph Labs, Inc. and Contributors
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

package x

import (
	"context"
	"fmt"

	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

func ParseJWT(jwtStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(jwtStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Errorf("unexpected signing method: %v",
				token.Header["alg"])
		}
		return []byte(WorkerConfig.HmacSecret), nil
	})

	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse jwt token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.Errorf("claims in jwt token is not map claims")
	}
	return claims, nil
}

func ExtractUserName(jwtToken string) (string, error) {
	claims, err := ParseJWT(jwtToken)
	if err != nil {
		return "", err
	}
	userId, ok := claims["userid"].(string)
	if !ok {
		return "", errors.Errorf("userid in claims is not a string:%v", userId)
	}

	return userId, nil
}

func ExtractNamespaceFromJwt(jwtToken string) (uint64, error) {
	claims, err := ParseJWT(jwtToken)
	if err != nil {
		return 0, errors.Wrap(err, "extracting namespace from JWT")
	}
	namespace, ok := claims["namespace"].(float64)
	if !ok {
		return 0, errors.Errorf("namespace in claims is not valid:%v", namespace)
	}
	return uint64(namespace), nil
}

func ExtractNamespaceFrom(ctx context.Context) (uint64, error) {
	jwtString, err := ExtractJwt(ctx)
	if err != nil {
		return 0, fmt.Errorf("extracting namespace from JWT %w", err)
	}
	return ExtractNamespaceFromJwt(jwtString)
}
