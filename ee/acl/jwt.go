// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package acl

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type JwtHeader struct {
	Alg string // the jwt algorithm
	Typ string // the header type "JWT"
}

var StdJwtHeader = JwtHeader{
	Alg: "HS256",
	Typ: "JWT",
}

type JwtGroup struct {
	Group string
}

type JwtPayload struct {
	Userid string
	Exp    int64 // the unix time sinch epoch
	Groups []JwtGroup
}

type Jwt struct {
	Header  JwtHeader
	Payload JwtPayload
}

// convert the jwt to string in format xxx.yyy.zzz
// where xxx represents the header, yyy represents the payload, and zzz represents the
// HMAC SHA256 signature signed by the key
func (jwt *Jwt) EncodeToString(key []byte) (string, error) {
	if len(key) == 0 {
		return "", fmt.Errorf("the key should not be empty")
	}

	header, err := json.Marshal(jwt.Header)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(jwt.Payload)
	if err != nil {
		return "", err
	}

	// generate the signature
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write(header); err != nil {
		return "", err
	}
	if _, err := mac.Write(payload); err != nil {
		return "", err
	}
	signature := mac.Sum(nil)

	headerBase64 := base64.StdEncoding.EncodeToString(header)
	payloadBase64 := base64.StdEncoding.EncodeToString(payload)
	signatureBase64 := base64.StdEncoding.EncodeToString(signature)
	return headerBase64 + "." + payloadBase64 + "." + signatureBase64, nil
}

// Decode the input string into the current Jwt struct, and also verify
// that the signature in the input is valid using the key if checkSignature is true
func (jwt *Jwt) DecodeString(input string, key []byte) error {
	components := strings.Split(input, ".")
	if len(components) != 3 {
		return fmt.Errorf("Input is not in format xxx.yyy.zzz")
	}
	if len(key) == 0 {
		return fmt.Errorf("The key should not be empty")
	}

	header, err := base64.StdEncoding.DecodeString(components[0])
	if err != nil {
		return fmt.Errorf("Unable to base64 decode the header: %v", components[0])
	}
	payload, err := base64.StdEncoding.DecodeString(components[1])
	if err != nil {
		return fmt.Errorf("Unable to base64 decode the payload: %v", components[1])
	}
	signature, err := base64.StdEncoding.DecodeString(components[2])
	if err != nil {
		return fmt.Errorf("Unable to base64 decode the signature: %v", components[2])
	}

	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write(header); err != nil {
		return fmt.Errorf("Error while writing header to construct signature: %v", err)
	}
	if _, err := mac.Write(payload); err != nil {
		return fmt.Errorf("Error while writing payload to construct signature: %v", err)
	}
	expectedSignature := mac.Sum(nil)
	if !hmac.Equal(signature, expectedSignature) {
		return fmt.Errorf("JWT signature mismatch")
	}

	if err = json.Unmarshal(header, &jwt.Header); err != nil {
		return err
	}
	if err = json.Unmarshal(payload, &jwt.Payload); err != nil {
		return err
	}
	return nil
}
