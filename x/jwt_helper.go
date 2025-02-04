/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"context"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

var (
	errTokenExpired = errors.New("Token is expired")
)

// MaybeKeyToBytes converts the x.Sensitive type into []byte if the type of interface really
// is x.Sensitive. We keep the type x.Sensitive for private and public keys so that it
// doesn't get printed into the logs but the type the JWT library needs is []byte.
func MaybeKeyToBytes(k interface{}) interface{} {
	if kb, ok := k.(Sensitive); ok {
		return []byte(kb)
	}
	return k
}

func ParseJWT(jwtStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(jwtStr, func(token *jwt.Token) (interface{}, error) {
		if WorkerConfig.AclJwtAlg == nil {
			return nil, errors.Errorf("ACL is disabled")
		}
		if token.Method.Alg() != WorkerConfig.AclJwtAlg.Alg() {
			return nil, errors.Errorf("unexpected signing method in token: %v", token.Header["alg"])
		}
		return MaybeKeyToBytes(WorkerConfig.AclPublicKey), nil
	})
	if err != nil {
		// This is for backward compatibility in clients
		if errors.Is(err, jwt.ErrTokenExpired) {
			err = errors.Wrap(errTokenExpired, jwt.ErrTokenInvalidClaims.Error())
		}
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
