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

const publicKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----

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
=9fdK
-----END PGP PUBLIC KEY BLOCK-----`

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
