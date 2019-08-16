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
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
)

func enterpriseDetails(signedFile string, e *enterprise) error {
	publicKeyFile, err := os.Open(Zero.Conf.GetString("public_key"))
	if err != nil {
		return errors.Wrapf(err, "while opening public key file")
	}
	defer publicKeyFile.Close()

	entityList, err := openpgp.ReadArmoredKeyRing(publicKeyFile)
	if err != nil {
		return errors.Wrapf(err, "while reading public key")
	}

	sf, err := os.Open(signedFile)
	if err != nil {
		return errors.Wrapf(err, "while opening signed license file: %v", signedFile)
	}

	// The signed file is expected to be have ASCII encoding, so we have to decode it before
	// reading.
	b, err := armor.Decode(sf)
	if err != nil {
		return errors.Wrapf(err, "while decoding license file")
	}

	md, err := openpgp.ReadMessage(b.Body, entityList, nil, nil)
	if err != nil {
		return errors.Wrapf(err, "while reading PGP message from license file")
	}

	// We need to read the body for the signature verification check to happen.
	buf, err := ioutil.ReadAll(md.UnverifiedBody)
	if err != nil {
		return errors.Wrapf(err, "while reading body from signed license file")
	}
	if md.Signature == nil {
		return errors.New("invalid signature while trying to verify license file")
	}

	err = json.Unmarshal(buf, e)
	return errors.Wrapf(err, "while JSON unmarshaling body of license file")
}
