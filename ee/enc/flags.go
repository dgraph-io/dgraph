package enc

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

import (
	"github.com/dgraph-io/dgraph/ee/vault"
	"github.com/spf13/pflag"
)

// RegisterFlags registers the required encryption flags.
func RegisterFlags(flag *pflag.FlagSet) {
	flag.String("encryption_key_file", "",
		"[Enterprise Feature] Encryption At Rest: The file that stores the symmetric key of "+
			"length 16, 24, or 32 bytes. The key size determines the chosen AES cipher "+
			"(AES-128, AES-192, and AES-256 respectively).")

	vault.RegisterFlags(flag)
}
