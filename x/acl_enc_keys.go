/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"crypto"
	"crypto/ed25519"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/dgraph-io/ristretto/v2/z"
)

// Keys holds the configuration for ACL and encryption.
type Keys struct {
	AclJwtAlg         jwt.SigningMethod
	AclSecretKey      interface{}
	AclPublicKey      interface{}
	AclSecretKeyBytes Sensitive // we need this to compute hash for auth
	AclAccessTtl      time.Duration
	AclRefreshTtl     time.Duration
	EncKey            Sensitive
}

// GetEncAclKeys returns the ACL and encryption keys as configured by the user
// through the --acl, --encryption, and --vault flags.
func GetEncAclKeys(config *viper.Viper) (*Keys, error) {
	aclSuperFlag := z.NewSuperFlag(config.GetString("acl")).MergeAndCheckDefault(AclDefaults)
	encSuperFlag := z.NewSuperFlag(config.GetString("encryption")).MergeAndCheckDefault(EncDefaults)

	// Get SecretKey and EncKey from vault / acl / encryption SuperFlags
	aclKey, encKey := vaultGetKeys(config)

	encKeyFile := encSuperFlag.GetPath(flagEncKeyFile)
	if encKeyFile != "" {
		if encKey != nil {
			return nil, fmt.Errorf("flags: Encryption key set in both vault and encryption flags")
		}
		var err error
		if encKey, err = os.ReadFile(encKeyFile); err != nil {
			return nil, fmt.Errorf("error reading encryption key from file: %s: %s", encKeyFile, err)
		}
	}
	if l := len(encKey); encKey != nil && l != 16 && l != 32 && l != 64 {
		return nil, fmt.Errorf("encryption key must have length of 16, 32, or 64 bytes, got %d bytes instead", l)
	}

	aclSecretFile := aclSuperFlag.GetPath(flagAclKeyFile)
	if aclSecretFile != "" {
		if aclKey != nil {
			return nil, fmt.Errorf("flags: ACL secret key set in both vault and acl flags")
		}
		var err error
		if aclKey, err = os.ReadFile(aclSecretFile); err != nil {
			return nil, fmt.Errorf("error reading ACL secret key from file: %s: %s", aclSecretFile, err)
		}
	}

	keys := &Keys{
		AclSecretKeyBytes: aclKey,
		AclAccessTtl:      aclSuperFlag.GetDuration(flagAclAccessTtl),
		AclRefreshTtl:     aclSuperFlag.GetDuration(flagAclRefreshTtl),
		EncKey:            encKey,
	}

	if aclKey != nil {
		algStr := aclSuperFlag.GetString(flagAclJwtAlg)
		aclAlg := jwt.GetSigningMethod(algStr)
		if aclAlg == nil {
			return nil, fmt.Errorf("Unsupported JWT signing algorithm for ACL: %v", algStr)
		}
		if err := checkAclKeyLength(aclAlg, aclKey); err != nil {
			return nil, err
		}
		privKey, pubKey, err := parseJWTKey(aclAlg, aclKey)
		if err != nil {
			return nil, err
		}

		keys.AclJwtAlg = aclAlg
		keys.AclSecretKey = privKey
		keys.AclPublicKey = pubKey
	}

	return keys, nil
}

func parseJWTKey(alg jwt.SigningMethod, key Sensitive) (interface{}, interface{}, error) {
	switch {
	case strings.HasPrefix(alg.Alg(), "HS"):
		return key, key, nil

	case strings.HasPrefix(alg.Alg(), "ES"):
		pk, err := jwt.ParseECPrivateKeyFromPEM(key)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error parsing ACL key as ECDSA private key")
		}
		return pk, &pk.PublicKey, nil

	case strings.HasPrefix(alg.Alg(), "RS") || strings.HasPrefix(alg.Alg(), "PS"):
		pk, err := jwt.ParseRSAPrivateKeyFromPEM(key)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error parsing ACL key as RSA private key")
		}
		return pk, &pk.PublicKey, nil

	case alg.Alg() == "EdDSA":
		pk, err := jwt.ParseEdPrivateKeyFromPEM(key)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error parsing ACL key as EdDSA private key")
		}
		return pk.(crypto.Signer), pk.(ed25519.PrivateKey).Public(), nil

	default:
		return nil, nil, errors.Errorf("unsupported signing algorithm: %v", alg.Alg())
	}
}

func checkAclKeyLength(alg jwt.SigningMethod, key Sensitive) error {
	if !strings.HasPrefix(alg.Alg(), "HS") {
		return nil
	}

	sl, err := strconv.Atoi(strings.TrimPrefix(alg.Alg(), "HS"))
	if err != nil {
		return errors.Wrapf(err, "error finding sha length for algo %v", alg.Alg())
	}

	// SHA length has to be smaller or equal to the key length
	if sl > len(key)*8 {
		return errors.Errorf("ACL key length [%v <= %v] bits for JWT algorithm [%v]", len(key)*8, sl, alg.Alg())
	}
	return nil
}

func RegisterAclAndEncFlags(flag *pflag.FlagSet) {
	registerAclFlag(flag)
	registerEncFlag(flag)
	registerVaultFlag(flag, true, true)
}

func RegisterEncFlag(flag *pflag.FlagSet) {
	registerEncFlag(flag)
	registerVaultFlag(flag, false, true)
}

func registerAclFlag(flag *pflag.FlagSet) {
	helpText := z.NewSuperFlagHelp(AclDefaults).
		Head("ACL options").
		Flag("jwt-alg", "JWT signing algorithm, default HS256").
		Flag("secret-file",
			"The file that stores secret key or private key, which is used to sign the ACL JWT.").
		Flag("access-ttl",
			"The TTL for the access JWT.").
		Flag("refresh-ttl",
			"The TTL for the refresh JWT.").
		String()
	flag.String(flagAcl, AclDefaults, helpText)
}

func registerEncFlag(flag *pflag.FlagSet) {
	helpText := z.NewSuperFlagHelp(EncDefaults).
		Head("Encryption At Rest options").
		Flag("key-file", "The file that stores the symmetric key of length 16, 24, or 32 bytes."+
			"The key size determines the chosen AES cipher (AES-128, AES-192, and AES-256 respectively).").
		String()
	flag.String(flagEnc, EncDefaults, helpText)
}

func BuildEncFlag(filename string) string {
	return fmt.Sprintf("key-file=%s;", filename)
}
