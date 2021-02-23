// +build !oss

/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package enc

import (
	"encoding/base64"
	"io/ioutil"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

const (
	VaultDefaults = `addr=http://localhost:8200; path=secret/data/dgraph; field=enc_key; format=base64;`
)

// RegisterVaultFlags registers the required flags to integrate with Vault.
func registerVaultFlags(flag *pflag.FlagSet) {
	// The following are Vault options. Applicable for alpha, live, bulk, debug, restore sub-cmds
	flag.String("vault", VaultDefaults, z.NewSuperFlagHelp(VaultDefaults).
		Head("Vault options").
		Flag("addr",
			"Vault server address in the form of http://ip:port").
		Flag("role-id-file",
			"File containing Vault role-id used for approle auth.").
		Flag("secret-id-file",
			"File containing Vault secret-id used for approle auth.").
		Flag("path",
			"Vault kv store path. e.g. secret/data/dgraph for kv-v2, kv/dgraph for kv-v1.").
		Flag("field",
			"Vault kv store field whose value is the base64 encoded encryption key.").
		Flag("format",
			"Vault field format: raw or base64.").
		String())
}

// vaultKeyReader implements the KeyReader interface. It reads the key from vault server.
type vaultKeyReader struct {
	addr     string
	roleID   string
	secretID string
	path     string
	field    string
	format   string
}

func newVaultKeyReader(vaultFlag *z.SuperFlag) (*vaultKeyReader, error) {
	v := &vaultKeyReader{
		addr:     vaultFlag.GetString("addr"),
		roleID:   vaultFlag.GetString("role-id-file"),
		secretID: vaultFlag.GetString("secret-id-file"),
		path:     vaultFlag.GetString("path"),
		field:    vaultFlag.GetString("field"),
		format:   vaultFlag.GetString("format"),
	}

	if v.addr == "" || v.path == "" || v.field == "" || v.format == "" {
		return nil, errors.Errorf("%v, %v, %v or %v is missing",
			"addr", "path", "field", "format")
	}
	if v.format != "base64" && v.format != "raw" {
		return nil, errors.Errorf(`--vault "format = %v;" must be one of base64 or raw`, v.format)
	}

	if v.roleID != "" && v.secretID != "" {
		return v, nil
	}
	return nil, errors.Errorf("%v and %v must both be specified",
		"role-id-file", "secret-id-file")
}

// readKey reads the key from the vault kv store.
func (vkr *vaultKeyReader) readKey() (x.SensitiveByteSlice, error) {
	if vkr == nil {
		return nil, errors.Errorf("nil vaultKeyReader")
	}

	// Read the files.
	roleID, err := ioutil.ReadFile(vkr.roleID)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading role-id file (%v)", vkr.roleID)
	}
	secretID, err := ioutil.ReadFile(vkr.secretID)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading secret-id file (%v)", vkr.secretID)
	}

	// Get a Vault Client.
	vConfig := &api.Config{
		Address: vkr.addr,
	}
	client, err := api.NewClient(vConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "while creating a new vault client")
	}

	// Login into the Vault with approle Auth.
	data := map[string]interface{}{
		"role_id":   string(roleID),
		"secret_id": string(secretID),
	}
	resp, err := client.Logical().Write("auth/approle/login", data)
	if err != nil {
		return nil, errors.Wrapf(err, "while logging into vault")
	}
	if resp.Auth == nil {
		return nil, errors.Errorf("login response from vault is bad")
	}
	client.SetToken(resp.Auth.ClientToken)

	// Read from KV store. The given path must be v1 or v2 format. We use it as is.
	secret, err := client.Logical().Read(vkr.path)
	if err != nil || secret == nil {
		return nil, errors.Errorf("error or nil secret on reading key at %v: "+
			"err %v", vkr.path, err)
	}

	// Parse key from response
	var m map[string]interface{}
	m, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		glog.Infof("Unable to extract key from kv v2 response. Trying kv v1.")
		m = secret.Data
	}
	kVal, ok := m[vkr.field]
	if !ok {
		return nil, errors.Errorf("secret key not found at %v", vkr.field)
	}
	kbyte := []byte(kVal.(string))
	if vkr.format == "base64" {
		kbyte, err = base64.StdEncoding.DecodeString(kVal.(string))
		if err != nil {
			return nil, errors.Errorf("Unable to decode the Base64 Encoded key: err %v", err)
		}
	}
	// Validate key length suitable for AES.
	klen := len(kbyte)
	if klen != 16 && klen != 32 && klen != 64 {
		return nil, errors.Errorf("bad key length %v from vault", klen)
	}
	return kbyte, nil
}
