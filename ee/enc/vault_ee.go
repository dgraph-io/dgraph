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
	"github.com/golang/glog"
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Configuration options of Vault.
const (
	vaultAddr         = "vault_addr"
	vaultRoleIDFile   = "vault_roleid_file"
	vaultSecretIDFile = "vault_secretid_file"
	vaultPath         = "vault_path"
	vaultField        = "vault_field"
	vaultFormat       = "vault_format"
)

// RegisterVaultFlags registers the required flags to integrate with Vault.
func registerVaultFlags(flag *pflag.FlagSet) {
	// The following are Vault options. Applicable for alpha, live, bulk, debug, restore sub-cmds
	flag.String(vaultAddr, "http://localhost:8200",
		"Vault server's address in the form http://ip:port.")
	flag.String(vaultRoleIDFile, "",
		"File containing Vault role-id used for approle auth.")
	flag.String(vaultSecretIDFile, "",
		"File containing Vault secret-id used for approle auth.")
	flag.String(vaultPath, "secret/data/dgraph",
		"Vault kv store path. e.g. secret/data/dgraph for kv-v2, kv/dgraph for kv-v1.")
	flag.String(vaultField, "enc_key",
		"Vault kv store field whose value is the Base64 encoded encryption key.")
	flag.String(vaultFormat, "base64",
		"Vault field format. raw or base64")
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

func newVaultKeyReader(cfg *viper.Viper) (*vaultKeyReader, error) {
	v := &vaultKeyReader{
		addr:     cfg.GetString(vaultAddr),
		roleID:   cfg.GetString(vaultRoleIDFile),
		secretID: cfg.GetString(vaultSecretIDFile),
		path:     cfg.GetString(vaultPath),
		field:    cfg.GetString(vaultField),
		format:   cfg.GetString(vaultFormat),
	}

	if v.addr == "" || v.path == "" || v.field == "" || v.format == "" {
		return nil, errors.Errorf("%v, %v, %v or %v is missing",
			vaultAddr, vaultPath, vaultField, vaultFormat)
	}
	if v.format != "base64" && v.format != "raw" {
		return nil, errors.Errorf("vault_format = %v; must be one of base64 or raw", v.format)
	}

	if v.roleID != "" && v.secretID != "" {
		return v, nil
	}
	return nil, errors.Errorf("%v and %v must both be specified",
		vaultRoleIDFile, vaultSecretIDFile)
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
