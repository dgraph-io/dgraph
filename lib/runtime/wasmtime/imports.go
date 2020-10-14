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

package wasmtime

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"runtime"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	gssmrruntime "github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/scale"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/bytecodealliance/wasmtime-go"
)

var ctx gssmrruntime.Context

func ext_print_num(data int64) {
	logger.Trace("[ext_print_num] executing...")
	logger.Info("[ext_print_num]", "message", fmt.Sprintf("%d", data))
}

func ext_print_utf8(c *wasmtime.Caller, data, len int32) {
	logger.Trace("[ext_print_utf8] executing...")
	m := c.GetExport("memory").Memory()
	mem := m.UnsafeData()
	logger.Info("[ext_print_utf8]", "message", fmt.Sprintf("%s", mem[data:data+len]))
	runtime.KeepAlive(m)
}

func ext_malloc(c *wasmtime.Caller, size int32) int32 {
	logger.Trace("[ext_malloc] executing...")
	res, err := ctx.Allocator.Allocate(uint32(size))
	if err != nil {
		logger.Error("[ext_malloc]", "Error:", err)
	}
	return int32(res)
}

func ext_free(c *wasmtime.Caller, addr int32) {
	logger.Trace("[ext_free] executing...")
	err := ctx.Allocator.Deallocate(uint32(addr))
	if err != nil {
		logger.Error("[ext_free]", "error", err)
	}
}

func ext_twox_128(c *wasmtime.Caller, data, len, out int32) {
	logger.Trace("[ext_twox_128] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()
	logger.Trace("[ext_twox_128]", "hashing", fmt.Sprintf("%s", memory[data:data+len]))

	res, err := common.Twox128Hash(memory[data : data+len])
	if err != nil {
		logger.Trace("error hashing in ext_twox_128", "error", err)
	}

	copy(memory[out:out+16], res[0:16])
	runtime.KeepAlive(m)
}

func ext_get_storage_into(c *wasmtime.Caller, keyData, keyLen, valueData, valueLen, valueOffset int32) int32 {
	logger.Trace("[ext_get_storage_into] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	key := memory[keyData : keyData+keyLen]
	val, err := ctx.Storage.Get(key)
	if err != nil {
		logger.Warn("[ext_get_storage_into]", "err", err)
		ret := 1<<32 - 1
		return int32(ret)
	} else if val == nil {
		logger.Debug("[ext_get_storage_into]", "err", "value is nil")
		ret := 1<<32 - 1
		return int32(ret)
	}

	if len(val) > int(valueLen) {
		logger.Debug("[ext_get_storage_into]", "error", "value exceeds allocated buffer length")
		//return 0
	}

	copy(memory[valueData:valueData+valueLen], val[valueOffset:])
	runtime.KeepAlive(m)
	return int32(len(val[valueOffset:]))
}

func ext_set_storage(c *wasmtime.Caller, keyData, keyLen, valueData, valueLen int32) {
	logger.Trace("[ext_set_storage] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	key := memory[keyData : keyData+keyLen]
	val := memory[valueData : valueData+valueLen]
	logger.Trace("[ext_set_storage]", "key", fmt.Sprintf("0x%x", key), "val", val)
	err := ctx.Storage.Set(key, val)
	if err != nil {
		logger.Error("[ext_set_storage]", "error", err)
		return
	}
}

func ext_storage_root(c *wasmtime.Caller, resultPtr int32) {
	logger.Trace("[ext_storage_root] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	root, err := ctx.Storage.Root()
	if err != nil {
		logger.Error("[ext_storage_root]", "error", err)
		return
	}

	copy(memory[resultPtr:resultPtr+32], root[:])
	runtime.KeepAlive(m)
}

func ext_get_allocated_storage(c *wasmtime.Caller, keyData, keyLen, writtenOut int32) int32 {
	logger.Trace("[ext_get_allocated_storage] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	key := memory[keyData : keyData+keyLen]
	logger.Trace("[ext_get_allocated_storage]", "key", fmt.Sprintf("0x%x", key))

	val, err := ctx.Storage.Get(key)
	if err != nil {
		logger.Error("[ext_get_allocated_storage]", "error", err)
		copy(memory[writtenOut:writtenOut+4], []byte{0xff, 0xff, 0xff, 0xff})
		return 0
	}

	if len(val) >= (1 << 32) {
		logger.Error("[ext_get_allocated_storage]", "error", "retrieved value length exceeds 2^32")
		copy(memory[writtenOut:writtenOut+4], []byte{0xff, 0xff, 0xff, 0xff})
		return 0
	}

	if val == nil {
		logger.Trace("[ext_get_allocated_storage]", "value", "nil")
		copy(memory[writtenOut:writtenOut+4], []byte{0xff, 0xff, 0xff, 0xff})
		return 0
	}

	ptr, err := ctx.Allocator.Allocate(uint32(len(val)))
	if err != nil {
		logger.Error("[ext_get_allocated_storage]", "error", err)
		copy(memory[writtenOut:writtenOut+4], []byte{0xff, 0xff, 0xff, 0xff})
		return 0
	}

	logger.Trace("[ext_get_allocated_storage]", "value", val)
	copy(memory[ptr:ptr+uint32(len(val))], val)

	// copy length to memory
	byteLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(byteLen, uint32(len(val)))

	// writtenOut stores the location of the memory that was allocated
	copy(memory[writtenOut:writtenOut+4], byteLen)

	runtime.KeepAlive(m)
	return int32(ptr)
}

func ext_clear_storage(c *wasmtime.Caller, keyData, keyLen int32) {
	logger.Trace("[ext_clear_storage] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	key := memory[keyData : keyData+keyLen]
	err := ctx.Storage.Delete(key)
	if err != nil {
		logger.Error("[ext_clear_storage]", "error", err)
	}

	runtime.KeepAlive(memory)
}

func ext_clear_prefix(c *wasmtime.Caller, prefixData, prefixLen int32) {
	logger.Trace("[ext_clear_prefix] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	prefix := memory[prefixData : prefixData+prefixLen]
	entries := ctx.Storage.Entries()
	for k := range entries {
		if bytes.Equal([]byte(k)[:prefixLen], prefix) {
			err := ctx.Storage.Delete([]byte(k))
			if err != nil {
				logger.Error("[ext_clear_prefix]", "err", err)
			}
		}
	}

	runtime.KeepAlive(memory)
}

func ext_blake2_256(c *wasmtime.Caller, data, length, out int32) {
	logger.Trace("[ext_blake2_256] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	hash, err := common.Blake2bHash(memory[data : data+length])
	if err != nil {
		logger.Error("[ext_blake2_256]", "error", err)
		return
	}

	copy(memory[out:out+32], hash[:])
	runtime.KeepAlive(memory)
}

func ext_blake2_256_enumerated_trie_root(c *wasmtime.Caller, valuesData, lensData, lensLen, result int32) {
	logger.Trace("[ext_blake2_256_enumerated_trie_root] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	t := trie.NewEmptyTrie()
	var i int32
	var pos int32 = 0
	for i = 0; i < lensLen; i++ {
		valueLenBytes := memory[lensData+i*4 : lensData+(i+1)*4]
		valueLen := int32(binary.LittleEndian.Uint32(valueLenBytes))
		value := memory[valuesData+pos : valuesData+pos+valueLen]
		logger.Trace("[ext_blake2_256_enumerated_trie_root]", "key", i, "value", fmt.Sprintf("%d", value), "valueLen", valueLen)
		pos += valueLen

		// encode the key
		encodedOutput, err := scale.Encode(big.NewInt(int64(i)))
		if err != nil {
			logger.Error("[ext_blake2_256_enumerated_trie_root]", "error", err)
			return
		}
		logger.Trace("[ext_blake2_256_enumerated_trie_root]", "key", i, "key value", encodedOutput)
		err = t.Put(encodedOutput, value)
		if err != nil {
			logger.Error("[ext_blake2_256_enumerated_trie_root]", "error", err)
			return
		}
	}
	root, err := t.Hash()
	if err != nil {
		logger.Error("[ext_blake2_256_enumerated_trie_root]", "error", err)
		return
	}
	logger.Trace("[ext_blake2_256_enumerated_trie_root]", "root", root)
	copy(memory[result:result+32], root[:])
	runtime.KeepAlive(memory)
}

func ext_print_hex(c *wasmtime.Caller, offset, size int32) {
	logger.Trace("[ext_print_hex] executing...")
}

func ext_storage_changes_root(c *wasmtime.Caller, a, b, d int32) int32 {
	logger.Trace("[ext_storage_changes_root] executing...")
	return 0
}

func ext_set_child_storage(c *wasmtime.Caller, storageKeyData, storageKeyLen, keyData, keyLen, valueData, valueLen int32) {
	logger.Trace("[ext_set_child_storage] executing...")
}

func ext_twox_64(c *wasmtime.Caller, data, length, out int32) {
	logger.Trace("[ext_twox_64] executing...")
}

func ext_sr25519_generate(c *wasmtime.Caller, idData, seed, seedLen, out int32) {
	logger.Trace("[ext_sr25519_generate] executing...")
}

func ext_sr25519_public_keys(c *wasmtime.Caller, idData, resultLen int32) int32 {
	logger.Trace("[ext_sr25519_public_keys] executing...")
	return 0
}

func ext_sr25519_sign(c *wasmtime.Caller, idData, pubkeyData, msgData, msgLen, out int32) int32 {
	logger.Trace("[ext_sr25519_sign] executing...")
	return 0
}

func ext_sr25519_verify(c *wasmtime.Caller, msgData, msgLen, sigData, pubkeyData int32) int32 {
	logger.Trace("[ext_sr25519_verify] executing...")
	return 0
}

func ext_ed25519_generate(c *wasmtime.Caller, idData, seed, seedLen, out int32) {
	logger.Trace("[ext_ed25519_generate] executing...")
}

func ext_ed25519_verify(c *wasmtime.Caller, msgData, msgLen, sigData, pubkeyData int32) int32 {
	logger.Trace("[ext_ed25519_verify] executing...")
	return 0
}

func ext_is_validator(c *wasmtime.Caller) int32 {
	logger.Trace("[ext_is_validator] executing...")
	return 0
}

func ext_local_storage_get(c *wasmtime.Caller, kind, key, keyLen, valueLen int32) int32 {
	logger.Trace("[ext_local_storage_get] executing...")
	return 0
}

func ext_local_storage_compare_and_set(c *wasmtime.Caller, kind, key, keyLen, oldValue, oldValueLen, newValue, newValueLen int32) int32 {
	logger.Trace("[ext_local_storage_compare_and_set] executing...")
	return 0
}

func ext_network_state(c *wasmtime.Caller, writtenOut int32) int32 {
	logger.Trace("[ext_network_state] executing...")
	return 0
}

func ext_submit_transaction(c *wasmtime.Caller, data, len int32) int32 {
	logger.Trace("[ext_submit_transaction] executing...")
	return 0
}

func ext_local_storage_set(c *wasmtime.Caller, kind, key, keyLen, value, valueLen int32) {
	logger.Trace("[ext_local_storage_set] executing...")
}

func ext_kill_child_storage(c *wasmtime.Caller, a, b int32) {
	logger.Trace("[ext_kill_child_storage] executing...")
}

func ext_sandbox_memory_new(c *wasmtime.Caller, a, b int32) int32 {
	logger.Trace("[ext_sandbox_memory_new] executing...")
	return 0
}

func ext_sandbox_memory_teardown(c *wasmtime.Caller, a int32) {
	logger.Trace("[ext_sandbox_memory_teardown] executing...")
}

func ext_sandbox_instantiate(c *wasmtime.Caller, a, b, g, d, e, f int32) int32 {
	logger.Trace("[ext_sandbox_instantiate] executing...")
	return 0
}

func ext_sandbox_invoke(c *wasmtime.Caller, a, b, i, d, e, f, g, h int32) int32 {
	logger.Trace("[ext_sandbox_invoke] executing...")
	return 0
}

func ext_sandbox_instance_teardown(c *wasmtime.Caller, a int32) {
	logger.Trace("[ext_sandbox_instance_teardown] executing...")
}

func ext_get_allocated_child_storage(c *wasmtime.Caller, a, b, i, d, e int32) int32 {
	logger.Trace("[ext_get_allocated_child_storage] executing...")
	return 0
}

func ext_child_storage_root(c *wasmtime.Caller, a, b, i int32) int32 {
	logger.Trace("[ext_child_storage_root] executing...")
	return 0
}

func ext_clear_child_storage(c *wasmtime.Caller, a, b, d, z int32) {
	logger.Trace("[ext_clear_child_storage] executing...")
}

func ext_secp256k1_ecdsa_recover_compressed(c *wasmtime.Caller, a, b, i int32) int32 {
	logger.Trace("[ext_secp256k1_ecdsa_recover_compressed] executing...")
	return 0
}

func ext_sandbox_memory_get(c *wasmtime.Caller, a, b, d, z int32) int32 {
	logger.Trace("[ext_sandbox_memory_get] executing...")
	return 0
}

func ext_sandbox_memory_set(c *wasmtime.Caller, a, b, d, z int32) int32 {
	logger.Trace("[ext_sandbox_memory_set] executing...")
	return 0
}

func ext_log(c *wasmtime.Caller, a, b, d, e, z int32) {
	logger.Trace("[ext_log] executing...")
}

func ext_twox_256(c *wasmtime.Caller, data, len, out int32) {
	logger.Trace("[ext_twox_256] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()
	logger.Trace("[ext_twox_256] hashing...", "value", fmt.Sprintf("%s", memory[data:data+len]))

	hash, err := common.Twox256(memory[data : data+len])
	if err != nil {
		logger.Error("[ext_twox_256]", "error", err)
		return
	}

	copy(memory[out:out+32], hash[:])
	runtime.KeepAlive(m)
}

func ext_blake2_128(c *wasmtime.Caller, data, length, out int32) {
	logger.Trace("[ext_blake2_128] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	hash, err := common.Blake2b128(memory[data : data+length])
	if err != nil {
		logger.Error("[ext_blake2_128]", "error", err)
		return
	}

	logger.Trace("[ext_blake2_128]", "hash", fmt.Sprintf("0x%x", hash))
	copy(memory[out:out+16], hash[:])
	runtime.KeepAlive(m)
}

func ext_keccak_256(c *wasmtime.Caller, data, length, out int32) {
	logger.Trace("[ext_keccak_256] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	hash, err := common.Keccak256(memory[data : data+length])
	if err != nil {
		logger.Error("[ext_keccak_256]", "error", err)
		return
	}

	logger.Trace("[ext_keccak_256]", "hash", hash)
	copy(memory[out:out+32], hash[:])
	runtime.KeepAlive(m)
}

func ext_exists_storage(c *wasmtime.Caller, a, b int32) int32 {
	logger.Trace("[ext_exists_storage] executing...")
	logger.Warn("[ext_exists_storage] not yet implemented")
	return 0
}

func ext_exists_child_storage(c *wasmtime.Caller, a, b, e, d int32) int32 {
	logger.Trace("[ext_exists_child_storage] executing...")
	logger.Warn("[ext_exists_child_storage] not yet implemented")
	return 0
}

func ext_clear_child_prefix(c *wasmtime.Caller, a, b, e, d int32) {
	logger.Trace("[ext_clear_child_prefix] executing...")
	logger.Warn("[ext_clear_child_prefix] not yet implemented")
}

func ext_get_child_storage_into(c *wasmtime.Caller, storageKeyData, storageKeyLen, keyData, keyLen, valueData, valueLen, valueOffset int32) int32 {
	logger.Trace("[ext_get_child_storage_into] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	keyToChild := memory[storageKeyData : storageKeyData+storageKeyLen]
	key := memory[keyData : keyData+keyLen]

	value, err := ctx.Storage.GetChildStorage(keyToChild, key)
	if err != nil {
		logger.Error("[ext_get_child_storage_into]", "error", err)
		return -(1 << 31)
	}

	copy(memory[valueData:valueData+valueLen], value[valueOffset:])
	runtime.KeepAlive(m)
	return int32(len(value[valueOffset:]))
}

func ext_ed25519_public_keys(c *wasmtime.Caller, idData, resultLen int32) int32 {
	logger.Trace("[ext_ed25519_public_keys] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	keys := ctx.Keystore.Ed25519PublicKeys()
	// TODO: when do deallocate?
	offset, err := ctx.Allocator.Allocate(uint32(len(keys) * 32))
	if err != nil {
		logger.Error("[ext_ed25519_public_keys]", "error", err)
		return -1
	}

	for i, key := range keys {
		copy(memory[offset+uint32(i*32):offset+uint32((i+1)*32)], key.Encode())
	}

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(len(keys)))
	copy(memory[resultLen:resultLen+4], buf)
	runtime.KeepAlive(m)
	return int32(offset)
}

func ext_ed25519_sign(c *wasmtime.Caller, idData, pubkeyData, msgData, msgLen, out int32) int32 {
	logger.Trace("[ext_ed25519_sign] executing...")
	m := c.GetExport("memory").Memory()
	memory := m.UnsafeData()

	pubkeyBytes := memory[pubkeyData : pubkeyData+32]
	pubkey, err := ed25519.NewPublicKey(pubkeyBytes)
	if err != nil {
		logger.Error("[ext_ed25519_sign]", "error", err)
		return 1
	}

	signingKey := ctx.Keystore.GetKeypair(pubkey)
	if signingKey == nil {
		logger.Error("[ext_ed25519_sign] could not find key in keystore", "public key", pubkey)
		return 1
	}

	msgLenBytes := memory[msgLen : msgLen+4]
	msgLength := binary.LittleEndian.Uint32(msgLenBytes)
	msg := memory[msgData : msgData+int32(msgLength)]
	sig, err := signingKey.Sign(msg)
	if err != nil {
		logger.Error("[ext_ed25519_sign] could not sign message")
		return 1
	}

	copy(memory[out:out+64], sig)
	runtime.KeepAlive(m)
	return 0
}

// ImportsNodeRuntime returns the wasmtime imports for NODE_RUNTIME
func ImportsNodeRuntime(store *wasmtime.Store) []*wasmtime.Extern {
	ext_print_num := wasmtime.WrapFunc(store, ext_print_num)
	ext_malloc := wasmtime.WrapFunc(store, ext_malloc)
	ext_free := wasmtime.WrapFunc(store, ext_free)
	ext_print_utf8 := wasmtime.WrapFunc(store, ext_print_utf8)
	ext_print_hex := wasmtime.WrapFunc(store, ext_print_hex)
	ext_get_storage_into := wasmtime.WrapFunc(store, ext_get_storage_into)
	ext_set_storage := wasmtime.WrapFunc(store, ext_set_storage)
	ext_set_child_storage := wasmtime.WrapFunc(store, ext_set_child_storage)
	ext_storage_root := wasmtime.WrapFunc(store, ext_storage_root)
	ext_storage_changes_root := wasmtime.WrapFunc(store, ext_storage_changes_root)
	ext_get_allocated_storage := wasmtime.WrapFunc(store, ext_get_allocated_storage)
	ext_clear_storage := wasmtime.WrapFunc(store, ext_clear_storage)
	ext_clear_prefix := wasmtime.WrapFunc(store, ext_clear_prefix)
	ext_blake2_256_enumerated_trie_root := wasmtime.WrapFunc(store, ext_blake2_256_enumerated_trie_root)
	ext_blake2_256 := wasmtime.WrapFunc(store, ext_blake2_256)
	ext_twox_64 := wasmtime.WrapFunc(store, ext_twox_64)
	ext_twox_128 := wasmtime.WrapFunc(store, ext_twox_128)
	ext_sr25519_generate := wasmtime.WrapFunc(store, ext_sr25519_generate)
	ext_sr25519_public_keys := wasmtime.WrapFunc(store, ext_sr25519_public_keys)
	ext_sr25519_sign := wasmtime.WrapFunc(store, ext_sr25519_sign)
	ext_sr25519_verify := wasmtime.WrapFunc(store, ext_sr25519_verify)
	ext_ed25519_generate := wasmtime.WrapFunc(store, ext_ed25519_generate)
	ext_ed25519_verify := wasmtime.WrapFunc(store, ext_ed25519_verify)
	ext_is_validator := wasmtime.WrapFunc(store, ext_is_validator)
	ext_local_storage_get := wasmtime.WrapFunc(store, ext_local_storage_get)
	ext_local_storage_compare_and_set := wasmtime.WrapFunc(store, ext_local_storage_compare_and_set)
	ext_network_state := wasmtime.WrapFunc(store, ext_network_state)
	ext_submit_transaction := wasmtime.WrapFunc(store, ext_submit_transaction)
	ext_local_storage_set := wasmtime.WrapFunc(store, ext_local_storage_set)
	ext_kill_child_storage := wasmtime.WrapFunc(store, ext_kill_child_storage)
	ext_sandbox_memory_new := wasmtime.WrapFunc(store, ext_sandbox_memory_new)
	ext_sandbox_memory_teardown := wasmtime.WrapFunc(store, ext_sandbox_memory_teardown)
	ext_sandbox_instantiate := wasmtime.WrapFunc(store, ext_sandbox_instantiate)
	ext_sandbox_invoke := wasmtime.WrapFunc(store, ext_sandbox_invoke)
	ext_sandbox_instance_teardown := wasmtime.WrapFunc(store, ext_sandbox_instance_teardown)
	ext_get_allocated_child_storage := wasmtime.WrapFunc(store, ext_get_allocated_child_storage)
	ext_child_storage_root := wasmtime.WrapFunc(store, ext_child_storage_root)
	ext_clear_child_storage := wasmtime.WrapFunc(store, ext_clear_child_storage)
	ext_secp256k1_ecdsa_recover_compressed := wasmtime.WrapFunc(store, ext_secp256k1_ecdsa_recover_compressed)
	ext_sandbox_memory_get := wasmtime.WrapFunc(store, ext_sandbox_memory_get)
	ext_sandbox_memory_set := wasmtime.WrapFunc(store, ext_sandbox_memory_set)
	ext_log := wasmtime.WrapFunc(store, ext_log)

	return []*wasmtime.Extern{
		ext_blake2_256.AsExtern(),
		ext_twox_128.AsExtern(),
		ext_clear_storage.AsExtern(),
		ext_set_storage.AsExtern(),
		ext_get_allocated_storage.AsExtern(),
		ext_get_storage_into.AsExtern(),
		ext_kill_child_storage.AsExtern(),
		ext_sandbox_memory_new.AsExtern(),
		ext_sandbox_memory_teardown.AsExtern(),
		ext_sandbox_instantiate.AsExtern(),
		ext_sandbox_invoke.AsExtern(),
		ext_sandbox_instance_teardown.AsExtern(),
		ext_print_utf8.AsExtern(),
		ext_print_hex.AsExtern(),
		ext_print_num.AsExtern(),
		ext_is_validator.AsExtern(),
		ext_local_storage_get.AsExtern(),
		ext_local_storage_compare_and_set.AsExtern(),
		ext_sr25519_public_keys.AsExtern(),
		ext_network_state.AsExtern(),
		ext_sr25519_sign.AsExtern(),
		ext_submit_transaction.AsExtern(),
		ext_local_storage_set.AsExtern(),
		ext_get_allocated_child_storage.AsExtern(),
		ext_ed25519_generate.AsExtern(),
		ext_sr25519_generate.AsExtern(),
		ext_child_storage_root.AsExtern(),
		ext_clear_prefix.AsExtern(),
		ext_storage_root.AsExtern(),
		ext_storage_changes_root.AsExtern(),
		ext_clear_child_storage.AsExtern(),
		ext_set_child_storage.AsExtern(),
		ext_secp256k1_ecdsa_recover_compressed.AsExtern(),
		ext_ed25519_verify.AsExtern(),
		ext_sr25519_verify.AsExtern(),
		ext_sandbox_memory_get.AsExtern(),
		ext_sandbox_memory_set.AsExtern(),
		ext_blake2_256_enumerated_trie_root.AsExtern(),
		ext_malloc.AsExtern(),
		ext_free.AsExtern(),
		ext_twox_64.AsExtern(),
		ext_log.AsExtern(),
	}
}

// ImportsHostAPITester returns the wasmtime imports for host API tester
func ImportsHostAPITester(store *wasmtime.Store) []*wasmtime.Extern {
	ext_print_num := wasmtime.WrapFunc(store, ext_print_num)
	ext_malloc := wasmtime.WrapFunc(store, ext_malloc)
	ext_free := wasmtime.WrapFunc(store, ext_free)
	ext_print_utf8 := wasmtime.WrapFunc(store, ext_print_utf8)
	ext_print_hex := wasmtime.WrapFunc(store, ext_print_hex)
	ext_get_storage_into := wasmtime.WrapFunc(store, ext_get_storage_into)
	ext_set_storage := wasmtime.WrapFunc(store, ext_set_storage)
	ext_set_child_storage := wasmtime.WrapFunc(store, ext_set_child_storage)
	ext_storage_root := wasmtime.WrapFunc(store, ext_storage_root)
	ext_storage_changes_root := wasmtime.WrapFunc(store, ext_storage_changes_root)
	ext_get_allocated_storage := wasmtime.WrapFunc(store, ext_get_allocated_storage)
	ext_clear_storage := wasmtime.WrapFunc(store, ext_clear_storage)
	ext_clear_prefix := wasmtime.WrapFunc(store, ext_clear_prefix)
	ext_blake2_256_enumerated_trie_root := wasmtime.WrapFunc(store, ext_blake2_256_enumerated_trie_root)
	ext_blake2_128 := wasmtime.WrapFunc(store, ext_blake2_128)
	ext_blake2_256 := wasmtime.WrapFunc(store, ext_blake2_256)
	ext_twox_64 := wasmtime.WrapFunc(store, ext_twox_64)
	ext_twox_128 := wasmtime.WrapFunc(store, ext_twox_128)
	ext_twox_256 := wasmtime.WrapFunc(store, ext_twox_256)
	ext_sr25519_generate := wasmtime.WrapFunc(store, ext_sr25519_generate)
	ext_sr25519_public_keys := wasmtime.WrapFunc(store, ext_sr25519_public_keys)
	ext_sr25519_sign := wasmtime.WrapFunc(store, ext_sr25519_sign)
	ext_sr25519_verify := wasmtime.WrapFunc(store, ext_sr25519_verify)
	ext_ed25519_generate := wasmtime.WrapFunc(store, ext_ed25519_generate)
	ext_ed25519_verify := wasmtime.WrapFunc(store, ext_ed25519_verify)
	ext_is_validator := wasmtime.WrapFunc(store, ext_is_validator)
	ext_local_storage_get := wasmtime.WrapFunc(store, ext_local_storage_get)
	ext_local_storage_compare_and_set := wasmtime.WrapFunc(store, ext_local_storage_compare_and_set)
	ext_network_state := wasmtime.WrapFunc(store, ext_network_state)
	ext_submit_transaction := wasmtime.WrapFunc(store, ext_submit_transaction)
	ext_local_storage_set := wasmtime.WrapFunc(store, ext_local_storage_set)
	ext_kill_child_storage := wasmtime.WrapFunc(store, ext_kill_child_storage)
	ext_sandbox_memory_new := wasmtime.WrapFunc(store, ext_sandbox_memory_new)
	ext_sandbox_memory_teardown := wasmtime.WrapFunc(store, ext_sandbox_memory_teardown)
	ext_sandbox_instantiate := wasmtime.WrapFunc(store, ext_sandbox_instantiate)
	ext_sandbox_invoke := wasmtime.WrapFunc(store, ext_sandbox_invoke)
	ext_sandbox_instance_teardown := wasmtime.WrapFunc(store, ext_sandbox_instance_teardown)
	ext_get_allocated_child_storage := wasmtime.WrapFunc(store, ext_get_allocated_child_storage)
	ext_child_storage_root := wasmtime.WrapFunc(store, ext_child_storage_root)
	ext_clear_child_storage := wasmtime.WrapFunc(store, ext_clear_child_storage)
	ext_secp256k1_ecdsa_recover_compressed := wasmtime.WrapFunc(store, ext_secp256k1_ecdsa_recover_compressed)
	ext_sandbox_memory_get := wasmtime.WrapFunc(store, ext_sandbox_memory_get)
	ext_sandbox_memory_set := wasmtime.WrapFunc(store, ext_sandbox_memory_set)
	ext_keccak_256 := wasmtime.WrapFunc(store, ext_keccak_256)
	ext_exists_storage := wasmtime.WrapFunc(store, ext_exists_storage)
	ext_exists_child_storage := wasmtime.WrapFunc(store, ext_exists_child_storage)
	ext_clear_child_prefix := wasmtime.WrapFunc(store, ext_clear_child_prefix)
	ext_get_child_storage_into := wasmtime.WrapFunc(store, ext_get_child_storage_into)
	ext_ed25519_public_keys := wasmtime.WrapFunc(store, ext_ed25519_public_keys)
	ext_ed25519_sign := wasmtime.WrapFunc(store, ext_ed25519_sign)

	return []*wasmtime.Extern{
		ext_malloc.AsExtern(),
		ext_free.AsExtern(),
		ext_sandbox_instantiate.AsExtern(),
		ext_sandbox_instance_teardown.AsExtern(),
		ext_sandbox_invoke.AsExtern(),
		ext_sandbox_memory_new.AsExtern(),
		ext_sandbox_memory_get.AsExtern(),
		ext_sandbox_memory_set.AsExtern(),
		ext_sandbox_memory_teardown.AsExtern(),
		ext_twox_64.AsExtern(),
		ext_twox_128.AsExtern(),
		ext_twox_256.AsExtern(),
		ext_blake2_128.AsExtern(),
		ext_blake2_256.AsExtern(),
		ext_keccak_256.AsExtern(),
		ext_print_utf8.AsExtern(),
		ext_print_hex.AsExtern(),
		ext_print_num.AsExtern(),
		ext_set_storage.AsExtern(),
		ext_set_child_storage.AsExtern(),
		ext_clear_child_storage.AsExtern(),
		ext_clear_storage.AsExtern(),
		ext_exists_storage.AsExtern(),
		ext_exists_child_storage.AsExtern(),
		ext_clear_prefix.AsExtern(),
		ext_clear_child_prefix.AsExtern(),
		ext_kill_child_storage.AsExtern(),
		ext_get_allocated_storage.AsExtern(),
		ext_get_allocated_child_storage.AsExtern(),
		ext_get_storage_into.AsExtern(),
		ext_get_child_storage_into.AsExtern(),
		ext_storage_root.AsExtern(),
		ext_child_storage_root.AsExtern(),
		ext_storage_changes_root.AsExtern(),
		ext_blake2_256_enumerated_trie_root.AsExtern(),
		ext_ed25519_public_keys.AsExtern(),
		ext_ed25519_verify.AsExtern(),
		ext_ed25519_generate.AsExtern(),
		ext_ed25519_sign.AsExtern(),
		ext_sr25519_public_keys.AsExtern(),
		ext_sr25519_verify.AsExtern(),
		ext_sr25519_generate.AsExtern(),
		ext_sr25519_sign.AsExtern(),
		ext_secp256k1_ecdsa_recover_compressed.AsExtern(),
		ext_is_validator.AsExtern(),
		ext_submit_transaction.AsExtern(),
		ext_network_state.AsExtern(),
		ext_local_storage_set.AsExtern(),
		ext_local_storage_get.AsExtern(),
		ext_local_storage_compare_and_set.AsExtern(),
	}
}
