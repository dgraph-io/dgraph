/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package zero

import (
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/pkg/errors"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
)

// verifySignature verifies the signature given a public key. It also JSON unmarshals the details
// of the license and stores them in l.
func verifySignature(signedFile, publicKey io.Reader, l *license) error {
	entityList, err := openpgp.ReadArmoredKeyRing(publicKey)
	if err != nil {
		return errors.Wrapf(err, "while reading public key")
	}

	// The signed file is expected to be have ASCII encoding, so we have to decode it before
	// reading.
	b, err := armor.Decode(signedFile)
	if err != nil {
		return errors.Wrapf(err, "while decoding license file")
	}

	md, err := openpgp.ReadMessage(b.Body, entityList, nil, nil)
	if err != nil {
		return errors.Wrapf(err, "while reading PGP message from license file")
	}

	// We need to read the body for the signature verification check to happen.
	// md.Signature would be non-nil after reading the body if the verification is successfull.
	buf, err := ioutil.ReadAll(md.UnverifiedBody)
	if err != nil {
		return errors.Wrapf(err, "while reading body from signed license file")
	}
	// This could be nil even if signature verification failed, so we also check Signature == nil
	// below.
	if md.SignatureError != nil {
		return errors.Wrapf(md.SignatureError,
			"signature error while trying to verify license file")
	}
	if md.Signature == nil {
		return errors.New("invalid signature while trying to verify license file")
	}

	err = json.Unmarshal(buf, l)
	if err != nil {
		return errors.Wrapf(err, "while JSON unmarshaling body of license file")
	}
	if l.User == "" || l.MaxNodes == 0 || l.Expiry.IsZero() {
		return errors.Errorf("invalid JSON data, fields shouldn't be zero: %+v\n", l)
	}
	return nil
}
