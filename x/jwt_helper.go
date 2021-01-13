package x

import (
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

func ExtractUserName(jwtToken string) (string, error) {
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Errorf("unexpected signing method: %v",
				token.Header["alg"])
		}
		return []byte(WorkerConfig.HmacSecret), nil
	})

	if err != nil {
		return "", errors.Wrapf(err, "unable to parse jwt token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", errors.Errorf("claims in jwt token is not map claims")
	}

	userId, ok := claims["userid"].(string)
	if !ok {
		return "", errors.Errorf("userid in claims is not a string:%v", userId)
	}

	return userId, nil
}
