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
43jJL0ZpKmjIPPYbx+4TjyF8T5cSCvE=
=wx6r
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
