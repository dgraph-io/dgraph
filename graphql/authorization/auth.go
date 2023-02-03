/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go/v4"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
	"gopkg.in/square/go-jose.v2"

	"github.com/dgraph-io/gqlparser/v2/gqlerror"
)

type ctxKey string

const (
	AuthJwtCtxKey  = ctxKey("authorizationJwt")
	AuthMetaHeader = "Dgraph.Authorization"
)

var (
	supportedAlgorithms = map[string]jwt.SigningMethod{
		jwt.SigningMethodRS256.Name: jwt.SigningMethodRS256,
		jwt.SigningMethodRS384.Name: jwt.SigningMethodRS384,
		jwt.SigningMethodRS512.Name: jwt.SigningMethodRS512,
		jwt.SigningMethodHS256.Name: jwt.SigningMethodHS256,
		jwt.SigningMethodHS384.Name: jwt.SigningMethodHS384,
		jwt.SigningMethodHS512.Name: jwt.SigningMethodHS512,
	}
)

type AuthMeta struct {
	VerificationKey string
	JWKUrl          string
	JWKUrls         []string
	jwkSet          []*jose.JSONWebKeySet
	expiryTime      []time.Time
	RSAPublicKey    *rsa.PublicKey `json:"-"` // Ignoring this field
	Header          string
	Namespace       string
	Algo            string
	SigningMethod   jwt.SigningMethod `json:"-"` // Ignoring this field
	Audience        []string
	httpClient      *http.Client
	ClosedByDefault bool
}

// Validate required fields.
func (a *AuthMeta) validate() error {
	var fields string

	// If JWKUrl/JWKUrls is provided, we don't expect (VerificationKey, Algo),
	// they are needed only if JWKUrl/JWKUrls is not present there.
	if len(a.JWKUrls) != 0 || a.JWKUrl != "" {

		// User cannot provide both JWKUrl and JWKUrls.
		if len(a.JWKUrls) != 0 && a.JWKUrl != "" {
			return fmt.Errorf("expecting either JWKUrl or JWKUrls, both were given")
		}

		if a.VerificationKey != "" || a.Algo != "" {
			return fmt.Errorf("expecting either JWKUrl/JWKUrls or (VerificationKey, Algo), both were given")
		}

		// Audience should be a required field if JWKUrl is provided.
		if len(a.Audience) == 0 {
			fields = " `Audience` "
		}
	} else {
		if a.VerificationKey == "" {
			fields = " `Verification key`/`JWKUrl`/`JWKUrls`"
		}

		if a.Algo == "" {
			fields += " `Algo`"
		}
	}

	if a.Header == "" {
		fields += " `Header`"
	}

	if a.Namespace == "" {
		fields += " `Namespace`"
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
		if err := meta.validate(); err != nil {
			return nil, err
		}

		if algoErr := meta.initSigningMethod(); algoErr != nil {
			return nil, algoErr
		}

		if meta.JWKUrl != "" {
			meta.JWKUrls = append(meta.JWKUrls, meta.JWKUrl)
			meta.JWKUrl = ""
		}

		if len(meta.JWKUrls) != 0 {
			meta.expiryTime = make([]time.Time, len(meta.JWKUrls))
			meta.jwkSet = make([]*jose.JSONWebKeySet, len(meta.JWKUrls))
		}
		return &meta, nil
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

	// authInfo with be like `Dgraph.Authorization ...`, we append prefix `# ` to authinfo
	// to make it work with the regex matching algorithm.
	authInfo = "# " + authInfo
	idx := authMetaRegex.FindAllStringSubmatchIndex(authInfo, -1)
	if len(idx) != 1 || len(idx[0]) != 12 ||
		!strings.HasPrefix(authInfo, authInfo[idx[0][0]:idx[0][1]]) {
		return nil, gqlerror.Errorf("Invalid `Dgraph.Authorization` format: %s", authInfo)
	}

	meta.Header = authInfo[idx[0][4]:idx[0][5]]
	meta.Namespace = authInfo[idx[0][6]:idx[0][7]]
	meta.Algo = authInfo[idx[0][8]:idx[0][9]]
	meta.VerificationKey = authInfo[idx[0][10]:idx[0][11]]

	if err := meta.initSigningMethod(); err != nil {
		return nil, err
	}

	return &meta, nil
}

func ParseAuthMeta(schema string) (*AuthMeta, error) {
	metaInfo, err := Parse(schema)
	if err != nil {
		return nil, err
	}

	if _, ok := metaInfo.SigningMethod.(*jwt.SigningMethodRSA); ok {
		// The jwt library internally uses `bytes.IndexByte(data, '\n')` to fetch new line and fails
		// if we have newline "\n" as ASCII value {92,110} instead of the actual ASCII value of 10.
		// To fix this we replace "\n" with new line's ASCII value.
		bytekey := bytes.ReplaceAll([]byte(metaInfo.VerificationKey), []byte{92, 110}, []byte{10})

		if metaInfo.RSAPublicKey, err = jwt.ParseRSAPublicKeyFromPEM(bytekey); err != nil {
			return nil, err
		}
	}

	return metaInfo, nil
}

func (a *AuthMeta) GetHeader() string {
	if a == nil {
		return ""
	}
	return a.Header
}

// AttachAuthorizationJwt adds any incoming JWT authorization data into the grpc context metadata.
func (a *AuthMeta) AttachAuthorizationJwt(ctx context.Context,
	header http.Header) (context.Context, error) {
	if a == nil {
		return ctx, nil
	}

	authHeaderVal := header.Get(a.Header)
	if authHeaderVal == "" {
		return ctx, nil
	}

	if strings.HasPrefix(strings.ToLower(authHeaderVal), "bearer ") {
		parts := strings.Split(authHeaderVal, " ")
		if len(parts) != 2 {
			return ctx, fmt.Errorf("invalid Bearer-formatted header value for JWT (%s)",
				authHeaderVal)
		}
		authHeaderVal = parts[1]
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}

	md.Append(string(AuthJwtCtxKey), authHeaderVal)
	ctx = metadata.NewIncomingContext(ctx, md)
	return ctx, nil
}

type CustomClaims struct {
	authMeta      *AuthMeta
	AuthVariables map[string]interface{}
	jwt.StandardClaims
}

// UnmarshalJSON unmarshalls the claims present in the JWT.
// It also adds standard claims to the `AuthVariables`. If
// there is an auth variable with name same as one of auth
// variable then the auth variable supersedes the standard claim.
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
	if authValue, ok := result[c.authMeta.Namespace]; ok {
		if authJson, ok := authValue.(string); ok {
			if err := json.Unmarshal([]byte(authJson), &c.AuthVariables); err != nil {
				return err
			}
		} else {
			c.AuthVariables, _ = authValue.(map[string]interface{})
		}
	}

	// `result` contains all the claims, delete the claim of the namespace mentioned
	// in the Authorization Header.
	delete(result, c.authMeta.Namespace)
	// add AuthVariables into the `result` map, Now it contains all the AuthVariables
	// and other claims present in the token.
	for k, v := range c.AuthVariables {
		result[k] = v
	}

	// update `AuthVariables` with `result` map
	c.AuthVariables = result
	return nil
}

func (c *CustomClaims) validateAudience() error {
	// If there's no audience claim, ignore
	if c.Audience == nil || len(c.Audience) == 0 {
		return nil
	}

	// If there is an audience claim, but no value provided, fail
	if c.authMeta.Audience == nil {
		return fmt.Errorf("audience value was expected but not provided")
	}

	var match = false
	for _, audStr := range c.Audience {
		for _, expectedAudStr := range c.authMeta.Audience {
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

func (a *AuthMeta) ExtractCustomClaims(ctx context.Context) (*CustomClaims, error) {
	if a == nil {
		return &CustomClaims{}, nil
	}
	// return CustomClaims containing jwt and authvariables.
	md, _ := metadata.FromIncomingContext(ctx)
	jwtToken := md.Get(string(AuthJwtCtxKey))
	if len(jwtToken) == 0 {
		if a.ClosedByDefault {
			return &CustomClaims{}, fmt.Errorf("a valid JWT is required but was not provided")
		} else {
			return &CustomClaims{}, nil
		}
	} else if len(jwtToken) > 1 {
		return nil, fmt.Errorf("invalid jwt auth token")
	}
	return a.validateJWTCustomClaims(jwtToken[0])
}

func GetJwtToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	jwtToken := md.Get(string(AuthJwtCtxKey))
	if len(jwtToken) != 1 {
		return ""
	}
	return jwtToken[0]
}

// validateThroughJWKUrl validates the JWT token against the given list of JWKUrls.
// It returns an error only if the token is not validated against even one of the
// JWKUrl.
func (a *AuthMeta) validateThroughJWKUrl(jwtStr string) (*jwt.Token, error) {
	var err error
	var token *jwt.Token
	for i := 0; i < len(a.JWKUrls); i++ {
		if a.isExpired(i) {
			err = a.refreshJWK(i)
			if err != nil {
				return nil, errors.Wrap(err, "while refreshing JWK from the URL")
			}
		}

		token, err =
			jwt.ParseWithClaims(jwtStr, &CustomClaims{authMeta: a}, func(token *jwt.Token) (interface{}, error) {
				kid := token.Header["kid"]
				if kid == nil {
					return nil, errors.Errorf("kid not present in JWT")
				}

				signingKeys := a.jwkSet[i].Key(kid.(string))
				if len(signingKeys) == 0 {
					return nil, errors.Errorf("Invalid kid")
				}
				return signingKeys[0].Key, nil
			}, jwt.WithoutAudienceValidation())

		if err == nil {
			return token, nil
		}
	}
	return nil, err
}

func (a *AuthMeta) validateJWTCustomClaims(jwtStr string) (*CustomClaims, error) {
	var token *jwt.Token
	var err error
	// Verification through JWKUrl
	if len(a.JWKUrls) != 0 {
		token, err = a.validateThroughJWKUrl(jwtStr)
	} else {
		if a.Algo == "" {
			return nil, fmt.Errorf(
				"jwt token cannot be validated because verification algorithm is not set")
		}

		// The JWT library supports comparison of `aud` in JWT against a single string. Hence, we
		// disable the `aud` claim verification at the library end using `WithoutAudienceValidation` and
		// use our custom validation function `validateAudience`.
		token, err =
			jwt.ParseWithClaims(jwtStr, &CustomClaims{authMeta: a}, func(token *jwt.Token) (interface{}, error) {
				algo, _ := token.Header["alg"].(string)
				if algo != a.Algo {
					return nil, errors.Errorf("unexpected signing method: Expected %s Found %s",
						a.Algo, algo)
				}

				switch a.SigningMethod.(type) {
				case *jwt.SigningMethodHMAC:
					return []byte(a.VerificationKey), nil
				case *jwt.SigningMethodRSA:
					return a.RSAPublicKey, nil
				}

				return nil, errors.Errorf("couldn't parse signing method from token header: %s", algo)
			}, jwt.WithoutAudienceValidation())
	}

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

// FetchJWKs fetches the JSON Web Key sets for the JWKUrls. It returns an error if
// the fetching of key is failed even for one of the JWKUrl.
func (a *AuthMeta) FetchJWKs() error {
	if len(a.JWKUrls) == 0 {
		return errors.Errorf("No JWKUrl supplied")
	}

	for i := 0; i < len(a.JWKUrls); i++ {
		err := a.FetchJWK(i)
		if err != nil {
			return err
		}
	}
	return nil
}

// FetchJWK fetches the JSON web Key set for the JWKUrl at a given index.
func (a *AuthMeta) FetchJWK(i int) error {
	if len(a.JWKUrls) <= i {
		return errors.Errorf("not enough JWKUrls")
	}

	req, err := http.NewRequest("GET", a.JWKUrls[i], nil)
	if err != nil {
		return err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	type JwkArray struct {
		JWKs []json.RawMessage `json:"keys"`
	}

	var jwkArray JwkArray
	err = json.Unmarshal(data, &jwkArray)
	if err != nil {
		return err
	}

	a.jwkSet[i] = &jose.JSONWebKeySet{Keys: make([]jose.JSONWebKey, len(jwkArray.JWKs))}
	for k, jwk := range jwkArray.JWKs {
		err = a.jwkSet[i].Keys[k].UnmarshalJSON(jwk)
		if err != nil {
			return err
		}
	}

	// Try to Parse the Remaining time in the expiry of signing keys
	// from the `max-age` directive in the `Cache-Control` Header
	var maxAge int64

	if resp.Header["Cache-Control"] != nil {
		maxAge, _ = ParseMaxAge(resp.Header["Cache-Control"][0])
	}

	if maxAge == 0 {
		a.expiryTime[i] = time.Time{}
	} else {
		a.expiryTime[i] = time.Now().Add(time.Duration(maxAge) * time.Second)
	}

	return nil
}

func (a *AuthMeta) refreshJWK(i int) error {
	var err error
	for i := 0; i < 3; i++ {
		err = a.FetchJWK(i)
		if err == nil {
			return nil
		}
		time.Sleep(10 * time.Second)
	}
	return err
}

// To check whether JWKs are expired or not
// if expiryTime is equal to 0 which means there
// is no expiry time of the JWKs, so it always
// returns false
func (a *AuthMeta) isExpired(i int) bool {
	if a.expiryTime[i].IsZero() {
		return false
	}
	return time.Now().After(a.expiryTime[i])
}

// initSigningMethod takes the current Algo value, validates it's a supported SigningMethod, then sets the SigningMethod
// field.
func (a *AuthMeta) initSigningMethod() error {
	// configurations using JWK URLs do not use signing methods.
	if len(a.JWKUrls) != 0 || a.JWKUrl != "" {
		return nil
	}

	signingMethod, ok := supportedAlgorithms[a.Algo]
	if !ok {
		arr := make([]string, 0, len(supportedAlgorithms))
		for k := range supportedAlgorithms {
			arr = append(arr, k)
		}

		return errors.Errorf(
			"invalid jwt algorithm: found %s, but supported options are: %s",
			a.Algo, strings.Join(arr, ","),
		)
	}

	a.SigningMethod = signingMethod

	return nil
}

func (a *AuthMeta) InitHttpClient() {
	a.httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}
}
