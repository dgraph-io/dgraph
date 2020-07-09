// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package runtime

// #include <stdlib.h>
//
// extern int32_t ext_malloc(void *context, int32_t size);
// extern void ext_free(void *context, int32_t addr);
// extern void ext_print_utf8(void *context, int32_t utf8_data, int32_t utf8_len);
// extern void ext_print_hex(void *context, int32_t data, int32_t len);
// extern int32_t ext_get_storage_into(void *context, int32_t keyData, int32_t keyLen, int32_t valueData, int32_t valueLen, int32_t valueOffset);
// extern void ext_set_storage(void *context, int32_t keyData, int32_t keyLen, int32_t valueData, int32_t valueLen);
// extern void ext_blake2_256(void *context, int32_t data, int32_t len, int32_t out);
// extern void ext_clear_storage(void *context, int32_t keyData, int32_t keyLen);
// extern void ext_twox_64(void *context, int32_t data, int32_t len, int32_t out);
// extern void ext_twox_128(void *context, int32_t data, int32_t len, int32_t out);
// extern int32_t ext_get_allocated_storage(void *context, int32_t keyData, int32_t keyLen, int32_t writtenOut);
// extern void ext_storage_root(void *context, int32_t resultPtr);
// extern int32_t ext_storage_changes_root(void *context, int32_t a, int32_t b, int32_t c);
// extern void ext_clear_prefix(void *context, int32_t prefixData, int32_t prefixLen);
// extern int32_t ext_sr25519_verify(void *context, int32_t msgData, int32_t msgLen, int32_t sigData, int32_t pubkeyData);
// extern int32_t ext_ed25519_verify(void *context, int32_t msgData, int32_t msgLen, int32_t sigData, int32_t pubkeyData);
// extern void ext_blake2_256_enumerated_trie_root(void *context, int32_t valuesData, int32_t lensData, int32_t lensLen, int32_t result);
// extern void ext_print_num(void *context, int64_t data);
// extern void ext_keccak_256(void *context, int32_t data, int32_t len, int32_t out);
// extern int32_t ext_secp256k1_ecdsa_recover(void *context, int32_t msgData, int32_t sigData, int32_t pubkeyData);
// extern void ext_blake2_128(void *context, int32_t data, int32_t len, int32_t out);
// extern int32_t ext_is_validator(void *context);
// extern int32_t ext_local_storage_get(void *context, int32_t kind, int32_t key, int32_t keyLen, int32_t valueLen);
// extern int32_t ext_local_storage_compare_and_set(void *context, int32_t kind, int32_t key, int32_t keyLen, int32_t oldValue, int32_t oldValueLen, int32_t newValue, int32_t newValueLen);
// extern int32_t ext_sr25519_public_keys(void *context, int32_t idData, int32_t resultLen);
// extern int32_t ext_ed25519_public_keys(void *context, int32_t idData, int32_t resultLen);
// extern int32_t ext_network_state(void *context, int32_t writtenOut);
// extern int32_t ext_sr25519_sign(void *context, int32_t idData, int32_t pubkeyData, int32_t msgData, int32_t msgLen, int32_t out);
// extern int32_t ext_ed25519_sign(void *context, int32_t idData, int32_t pubkeyData, int32_t msgData, int32_t msgLen, int32_t out);
// extern int32_t ext_submit_transaction(void *context, int32_t data, int32_t len);
// extern void ext_local_storage_set(void *context, int32_t kind, int32_t key, int32_t keyLen, int32_t value, int32_t valueLen);
// extern void ext_ed25519_generate(void *context, int32_t idData, int32_t seed, int32_t seedLen, int32_t out);
// extern void ext_sr25519_generate(void *context, int32_t idData, int32_t seed, int32_t seedLen, int32_t out);
// extern void ext_set_child_storage(void *context, int32_t storageKeyData, int32_t storageKeyLen, int32_t keyData, int32_t keyLen, int32_t valueData, int32_t valueLen);
// extern int32_t ext_get_child_storage_into(void *context, int32_t storageKeyData, int32_t storageKeyLen, int32_t keyData, int32_t keyLen, int32_t valueData, int32_t valueLen, int32_t valueOffset);
// extern void ext_kill_child_storage(void *context, int32_t a, int32_t b);
// extern int32_t ext_sandbox_memory_new(void *context, int32_t a, int32_t b);
// extern void ext_sandbox_memory_teardown(void *context, int32_t a);
// extern int32_t ext_sandbox_instantiate(void *context, int32_t a, int32_t b, int32_t c, int32_t d, int32_t e, int32_t f);
// extern int32_t ext_sandbox_invoke(void *context, int32_t a, int32_t b, int32_t c, int32_t d, int32_t e, int32_t f, int32_t g, int32_t h);
// extern void ext_sandbox_instance_teardown(void *context, int32_t a);
// extern int32_t ext_get_allocated_child_storage(void *context, int32_t a, int32_t b, int32_t c, int32_t d, int32_t e);
// extern int32_t ext_child_storage_root(void *context, int32_t a, int32_t b, int32_t c);
// extern void ext_clear_child_storage(void *context, int32_t a, int32_t b, int32_t c, int32_t d);
// extern int32_t ext_secp256k1_ecdsa_recover_compressed(void *context, int32_t a, int32_t b, int32_t c);
// extern int32_t ext_sandbox_memory_get(void *context, int32_t a, int32_t b, int32_t c, int32_t d);
// extern int32_t ext_sandbox_memory_set(void *context, int32_t a, int32_t b, int32_t c, int32_t d);
// extern void ext_log(void *context, int32_t a, int32_t b, int32_t c, int32_t d, int32_t e);
// extern void ext_twox_256(void *context, int32_t a, int32_t b, int32_t c);
// extern int32_t ext_exists_storage(void *context, int32_t a, int32_t b);
// extern int32_t ext_exists_child_storage(void *context, int32_t a, int32_t b, int32_t c, int32_t d);
// extern void ext_clear_child_prefix(void *context, int32_t a, int32_t b, int32_t c, int32_t d);
import "C"

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/OneOfOne/xxhash"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
)

//export ext_kill_child_storage
func ext_kill_child_storage(context unsafe.Pointer, a, b C.int32_t) {
	logger.Trace("[ext_kill_child_storage] executing...")
	logger.Warn("[ext_kill_child_storage] not yet implemented")
}

//export ext_sandbox_memory_new
func ext_sandbox_memory_new(context unsafe.Pointer, a, b C.int32_t) C.int32_t {
	logger.Trace("[ext_sandbox_memory_new] executing...")
	logger.Warn("[ext_sandbox_memory_new] not yet implemented")
	return 0
}

//export ext_sandbox_memory_teardown
func ext_sandbox_memory_teardown(context unsafe.Pointer, a C.int32_t) {
	logger.Trace("[ext_sandbox_memory_teardown] executing...")
	logger.Warn("[ext_sandbox_memory_teardown] not yet implemented")
}

//export ext_sandbox_instantiate
func ext_sandbox_instantiate(context unsafe.Pointer, a, b, c, d, e, f C.int32_t) C.int32_t {
	logger.Trace("[ext_sandbox_instantiate] executing...")
	logger.Warn("[ext_sandbox_instantiate] not yet implemented")
	return 0
}

//export ext_sandbox_invoke
func ext_sandbox_invoke(context unsafe.Pointer, a, b, c, d, e, f, g, h C.int32_t) C.int32_t {
	logger.Trace("[ext_sandbox_invoke] executing...")
	logger.Warn("[ext_sandbox_invoke] not yet implemented")
	return 0
}

//export ext_sandbox_instance_teardown
func ext_sandbox_instance_teardown(context unsafe.Pointer, a C.int32_t) {
	logger.Trace("[ext_sandbox_instance_teardown] executing...")
	logger.Warn("[ext_sandbox_instance_teardown] not yet implemented")
}

//export ext_get_allocated_child_storage
func ext_get_allocated_child_storage(context unsafe.Pointer, a, b, c, d, e C.int32_t) C.int32_t {
	logger.Trace("[ext_get_allocated_child_storage] executing...")
	logger.Warn("[ext_get_allocated_child_storage] not yet implemented")
	return 0
}

//export ext_child_storage_root
func ext_child_storage_root(context unsafe.Pointer, a, b, c C.int32_t) C.int32_t {
	logger.Trace("[ext_child_storage_root] executing...")
	logger.Warn("[ext_child_storage_root] not yet implemented")
	return 0
}

//export ext_clear_child_storage
func ext_clear_child_storage(context unsafe.Pointer, a, b, c, d C.int32_t) {
	logger.Trace("[ext_clear_child_storage] executing...")
	logger.Warn("[ext_clear_child_storage] not yet implemented")
}

//export ext_secp256k1_ecdsa_recover_compressed
func ext_secp256k1_ecdsa_recover_compressed(context unsafe.Pointer, a, b, c C.int32_t) C.int32_t {
	logger.Trace("[ext_secp256k1_ecdsa_recover_compressed] executing...")
	logger.Warn("[ext_secp256k1_ecdsa_recover_compressed] not yet implemented")
	return 0
}

//export ext_sandbox_memory_get
func ext_sandbox_memory_get(context unsafe.Pointer, a, b, c, d C.int32_t) C.int32_t {
	logger.Trace("[ext_sandbox_memory_get] executing...")
	logger.Warn("[ext_sandbox_memory_get] not yet implemented")
	return 0
}

//export ext_sandbox_memory_set
func ext_sandbox_memory_set(context unsafe.Pointer, a, b, c, d C.int32_t) C.int32_t {
	logger.Trace("[ext_sandbox_memory_set] executing...")
	logger.Warn("[ext_sandbox_memory_set] not yet implemented")
	return 0
}

//export ext_log
func ext_log(context unsafe.Pointer, a, b, c, d, e C.int32_t) {
	logger.Trace("[ext_log] executing...")
	logger.Warn("[ext_log] not yet implemented")
}

//export ext_twox_256
func ext_twox_256(context unsafe.Pointer, data, len, out C.int32_t) {
	logger.Trace("[ext_twox_256] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	logger.Trace("[ext_twox_256] hashing...", "value", fmt.Sprintf("%s", memory[data:data+len]))

	h0 := xxhash.NewS64(0)
	_, err := h0.Write(memory[data : data+len])
	if err != nil {
		logger.Error("[ext_twox_256]", "error", err)
		return
	}
	res0 := h0.Sum64()
	hash0 := make([]byte, 8)
	binary.LittleEndian.PutUint64(hash0, res0)

	h1 := xxhash.NewS64(1)
	_, err = h1.Write(memory[data : data+len])
	if err != nil {
		logger.Error("[ext_twox_256]", "error", err)
		return
	}
	res1 := h1.Sum64()
	hash1 := make([]byte, 8)
	binary.LittleEndian.PutUint64(hash1, res1)

	h2 := xxhash.NewS64(2)
	_, err = h2.Write(memory[data : data+len])
	if err != nil {
		logger.Error("[ext_twox_256]", "error", err)
		return
	}
	res2 := h2.Sum64()
	hash2 := make([]byte, 8)
	binary.LittleEndian.PutUint64(hash2, res2)

	h3 := xxhash.NewS64(3)
	_, err = h3.Write(memory[data : data+len])
	if err != nil {
		logger.Error("[ext_twox_256]", "error", err)
		return
	}
	res3 := h3.Sum64()
	hash3 := make([]byte, 8)
	binary.LittleEndian.PutUint64(hash3, res3)

	fin := append(append(append(hash0, hash1...), hash2...), hash3...)
	copy(memory[out:out+32], fin)
}

//export ext_exists_storage
func ext_exists_storage(context unsafe.Pointer, a, b C.int32_t) C.int32_t {
	logger.Trace("[ext_exists_storage] executing...")
	logger.Warn("[ext_exists_storage] not yet implemented")
	return 0
}

//export ext_exists_child_storage
func ext_exists_child_storage(context unsafe.Pointer, a, b, c, d C.int32_t) C.int32_t {
	logger.Trace("[ext_exists_child_storage] executing...")
	logger.Warn("[ext_exists_child_storage] not yet implemented")
	return 0
}

//export ext_clear_child_prefix
func ext_clear_child_prefix(context unsafe.Pointer, a, b, c, d C.int32_t) {
	logger.Trace("[ext_clear_child_prefix] executing...")
	logger.Warn("[ext_clear_child_prefix] not yet implemented")
}

// RegisterImports_NodeRuntime returns the wasm imports for the substrate v0.6.x node runtime
func RegisterImports_NodeRuntime() (*wasm.Imports, error) { //nolint
	imports, err := wasm.NewImports().Append("ext_malloc", ext_malloc, C.ext_malloc)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_free", ext_free, C.ext_free)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_print_utf8", ext_print_utf8, C.ext_print_utf8)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_print_hex", ext_print_hex, C.ext_print_hex)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_print_num", ext_print_num, C.ext_print_num)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_get_storage_into", ext_get_storage_into, C.ext_get_storage_into)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_get_allocated_storage", ext_get_allocated_storage, C.ext_get_allocated_storage)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_set_storage", ext_set_storage, C.ext_set_storage)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_blake2_256", ext_blake2_256, C.ext_blake2_256)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_blake2_256_enumerated_trie_root", ext_blake2_256_enumerated_trie_root, C.ext_blake2_256_enumerated_trie_root)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_clear_storage", ext_clear_storage, C.ext_clear_storage)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_clear_prefix", ext_clear_prefix, C.ext_clear_prefix)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_twox_128", ext_twox_128, C.ext_twox_128)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_storage_root", ext_storage_root, C.ext_storage_root)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_storage_changes_root", ext_storage_changes_root, C.ext_storage_changes_root)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sr25519_verify", ext_sr25519_verify, C.ext_sr25519_verify)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_ed25519_verify", ext_ed25519_verify, C.ext_ed25519_verify)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_keccak_256", ext_keccak_256, C.ext_keccak_256)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_secp256k1_ecdsa_recover", ext_secp256k1_ecdsa_recover, C.ext_secp256k1_ecdsa_recover)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_blake2_128", ext_blake2_128, C.ext_blake2_128)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_is_validator", ext_is_validator, C.ext_is_validator)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_local_storage_get", ext_local_storage_get, C.ext_local_storage_get)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_local_storage_compare_and_set", ext_local_storage_compare_and_set, C.ext_local_storage_compare_and_set)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_ed25519_public_keys", ext_ed25519_public_keys, C.ext_ed25519_public_keys)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sr25519_public_keys", ext_sr25519_public_keys, C.ext_sr25519_public_keys)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_network_state", ext_network_state, C.ext_network_state)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sr25519_sign", ext_sr25519_sign, C.ext_sr25519_sign)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_ed25519_sign", ext_ed25519_sign, C.ext_ed25519_sign)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_submit_transaction", ext_submit_transaction, C.ext_submit_transaction)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_local_storage_set", ext_local_storage_set, C.ext_local_storage_set)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_ed25519_generate", ext_ed25519_generate, C.ext_ed25519_generate)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sr25519_generate", ext_sr25519_generate, C.ext_sr25519_generate)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_twox_64", ext_twox_64, C.ext_twox_64)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_set_child_storage", ext_set_child_storage, C.ext_set_child_storage)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_get_child_storage_into", ext_get_child_storage_into, C.ext_get_child_storage_into)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_kill_child_storage", ext_kill_child_storage, C.ext_kill_child_storage)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sandbox_memory_new", ext_sandbox_memory_new, C.ext_sandbox_memory_new)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sandbox_memory_teardown", ext_sandbox_memory_teardown, C.ext_sandbox_memory_teardown)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sandbox_instantiate", ext_sandbox_instantiate, C.ext_sandbox_instantiate)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sandbox_invoke", ext_sandbox_invoke, C.ext_sandbox_invoke)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sandbox_instance_teardown", ext_sandbox_instance_teardown, C.ext_sandbox_instance_teardown)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_get_allocated_child_storage", ext_get_allocated_child_storage, C.ext_get_allocated_child_storage)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_child_storage_root", ext_child_storage_root, C.ext_child_storage_root)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_clear_child_storage", ext_clear_child_storage, C.ext_clear_child_storage)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_secp256k1_ecdsa_recover_compressed", ext_secp256k1_ecdsa_recover_compressed, C.ext_secp256k1_ecdsa_recover_compressed)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sandbox_memory_get", ext_sandbox_memory_get, C.ext_sandbox_memory_get)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_sandbox_memory_set", ext_sandbox_memory_set, C.ext_sandbox_memory_set)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_log", ext_log, C.ext_log)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_twox_256", ext_twox_256, C.ext_twox_256)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_exists_storage", ext_exists_storage, C.ext_exists_storage)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_exists_child_storage", ext_exists_child_storage, C.ext_exists_child_storage)
	if err != nil {
		return nil, err
	}
	_, err = imports.Append("ext_clear_child_prefix", ext_clear_child_prefix, C.ext_clear_child_prefix)
	if err != nil {
		return nil, err
	}

	return imports, nil
}
