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
import "C"

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"unsafe"

	"github.com/ChainSafe/gossamer/codec"

	common "github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/crypto"
	trie "github.com/ChainSafe/gossamer/trie"
	log "github.com/ChainSafe/log15"
	xxhash "github.com/OneOfOne/xxhash"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
)

//export ext_print_num
func ext_print_num(context unsafe.Pointer, data C.int64_t) {
	log.Trace("[ext_print_num] executing...")
	log.Debug("[ext_print_num]", "message", fmt.Sprintf("%d", data))
}

//export ext_malloc
func ext_malloc(context unsafe.Pointer, size int32) int32 {
	log.Trace("[ext_malloc] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	data := instanceContext.Data()
	runtimeCtx, ok := data.(*RuntimeCtx)
	if !ok {
		panic(fmt.Sprintf("%#v", data))
	}

	// Allocate memory
	res, err := runtimeCtx.allocator.Allocate(uint32(size))
	if err != nil {
		log.Error("[ext_malloc]", "Error:", err)
		panic(err)
	}

	return int32(res)
}

//export ext_free
func ext_free(context unsafe.Pointer, addr int32) {
	log.Trace("[ext_free] executing...")
	log.Trace("[ext_free]", "addr", addr)
	instanceContext := wasm.IntoInstanceContext(context)

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)

	// Deallocate memory
	err := runtimeCtx.allocator.Deallocate(uint32(addr))
	if err != nil {
		log.Error("[ext_free] Error:", "Error", err)
		panic(err)
	}
}

// prints string located in memory at location `offset` with length `size`
//export ext_print_utf8
func ext_print_utf8(context unsafe.Pointer, utf8_data, utf8_len int32) {
	log.Trace("[ext_print_utf8] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	log.Debug("[ext_print_utf8]", "message", fmt.Sprintf("%s", memory[utf8_data:utf8_data+utf8_len]))
}

// prints hex formatted bytes located in memory at location `offset` with length `size`
//export ext_print_hex
func ext_print_hex(context unsafe.Pointer, offset, size int32) {
	log.Trace("[ext_print_hex] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	log.Debug("[ext_print_hex]", "message", fmt.Sprintf("%x", memory[offset:offset+size]))
}

// gets the key stored at memory location `keyData` with length `keyLen` and stores the value in memory at
// location `valueData`. the value can have up to value `valueLen` and the returned value starts at value[valueOffset:]
//export ext_get_storage_into
func ext_get_storage_into(context unsafe.Pointer, keyData, keyLen, valueData, valueLen, valueOffset int32) int32 {
	log.Trace("[ext_get_storage_into] executing...")

	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)
	t := runtimeCtx.trie

	key := memory[keyData : keyData+keyLen]
	val, err := t.Get(key)
	if err != nil {
		log.Error("[ext_get_storage_into]", "err", err)
		ret := 1<<32 - 1
		return int32(ret)
	} else if val == nil {
		log.Error("[ext_get_storage_into]", "err", "value is nil")
		ret := 1<<32 - 1
		return int32(ret)
	}

	if len(val) > int(valueLen) {
		log.Error("[ext_get_storage_into]", "error", "value exceeds allocated buffer length")
		return 0
	}

	copy(memory[valueData:valueData+valueLen], val[valueOffset:])
	return int32(len(val[valueOffset:]))
}

// puts the key at memory location `keyData` with length `keyLen` and value at memory location `valueData`
// with length `valueLen` into the storage trie
//export ext_set_storage
func ext_set_storage(context unsafe.Pointer, keyData, keyLen, valueData, valueLen int32) {
	log.Trace("[ext_set_storage] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)
	t := runtimeCtx.trie

	key := memory[keyData : keyData+keyLen]
	val := memory[valueData : valueData+valueLen]
	log.Trace("[ext_set_storage]", "key", key, "val", val)
	err := t.Put(key, val)
	if err != nil {
		log.Error("[ext_set_storage]", "error", err)
	}
}

//export ext_set_child_storage
func ext_set_child_storage(context unsafe.Pointer, storageKeyData, storageKeyLen, keyData, keyLen, valueData, valueLen int32) {
	log.Trace("[ext_set_child_storage] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)
	t := runtimeCtx.trie

	keyToChild := memory[storageKeyData : storageKeyData+storageKeyLen]
	key := memory[keyData : keyData+keyLen]
	value := memory[valueData : valueData+valueLen]

	err := t.PutIntoChild(keyToChild, key, value)
	if err != nil {
		log.Error("[ext_set_child_storage]", "error", err)
	}
}

//export ext_get_child_storage_into
func ext_get_child_storage_into(context unsafe.Pointer, storageKeyData, storageKeyLen, keyData, keyLen, valueData, valueLen, valueOffset int32) int32 {
	log.Trace("[ext_get_child_storage_into] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)
	t := runtimeCtx.trie

	keyToChild := memory[storageKeyData : storageKeyData+storageKeyLen]
	key := memory[keyData : keyData+keyLen]

	value, err := t.GetFromChild(keyToChild, key)
	if err != nil {
		log.Error("[ext_get_child_storage_into]", "error", err)
		return -(1 << 31)
	}

	copy(memory[valueData:valueData+valueLen], value[valueOffset:])
	return int32(len(value[valueOffset:]))
}

// returns the trie root in the memory location `resultPtr`
//export ext_storage_root
func ext_storage_root(context unsafe.Pointer, resultPtr int32) {
	log.Trace("[ext_storage_root] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)
	t := runtimeCtx.trie

	root, err := t.Hash()
	if err != nil {
		log.Error("[ext_storage_root]", "error", err)
		return
	}

	copy(memory[resultPtr:resultPtr+32], root[:])
}

//export ext_storage_changes_root
func ext_storage_changes_root(context unsafe.Pointer, a, b, c int32) int32 {
	log.Trace("[ext_storage_changes_root] executing...")
	log.Warn("[ext_storage_changes_root] Not yet implemented.")
	return 0
}

// gets value stored at key at memory location `keyData` with length `keyLen` and returns the location
// in memory where it's stored and stores its length in `writtenOut`
//export ext_get_allocated_storage
func ext_get_allocated_storage(context unsafe.Pointer, keyData, keyLen, writtenOut int32) int32 {
	log.Trace("[ext_get_allocated_storage] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)
	t := runtimeCtx.trie

	key := memory[keyData : keyData+keyLen]
	val, err := t.Get(key)
	if err == nil && len(val) >= (1<<32) {
		err = errors.New("retrieved value length exceeds 2^32")
	}

	if err != nil {
		log.Error("[ext_get_allocated_storage]", "error", err)
		return 0
	}

	// writtenOut stores the location of the 4 bytes of memory that was allocated
	var lenPtr int32 = 1
	memory[writtenOut] = byte(lenPtr)
	if val == nil {
		copy(memory[lenPtr:lenPtr+4], []byte{0xff, 0xff, 0xff, 0xff})
		return 0
	}

	// copy value to memory
	var ptr int32 = lenPtr + 4
	copy(memory[ptr:ptr+int32(len(val))], val)

	// copy length to memory
	byteLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(byteLen, uint32(len(val)))
	copy(memory[lenPtr:lenPtr+4], byteLen)

	// return ptr to value
	return ptr
}

// deletes the trie entry with key at memory location `keyData` with length `keyLen`
//export ext_clear_storage
func ext_clear_storage(context unsafe.Pointer, keyData, keyLen int32) {
	log.Trace("[ext_clear_storage] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)
	t := runtimeCtx.trie

	key := memory[keyData : keyData+keyLen]
	err := t.Delete(key)
	if err != nil {
		log.Error("[ext_storage_root]", "error", err)
	}
}

// deletes all entries in the trie that have a key beginning with the prefix stored at `prefixData`
//export ext_clear_prefix
func ext_clear_prefix(context unsafe.Pointer, prefixData, prefixLen int32) {
	log.Trace("[ext_clear_prefix] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)
	t := runtimeCtx.trie

	prefix := memory[prefixData : prefixData+prefixLen]
	entries := t.Entries()
	for k := range entries {
		if bytes.Equal([]byte(k)[:prefixLen], prefix) {
			err := t.Delete([]byte(k))
			if err != nil {
				log.Error("[ext_clear_prefix]", "err", err)
			}
		}
	}
}

// accepts an array of values, puts them into a trie, and returns the root
// the keys to the values are their position in the array
//export ext_blake2_256_enumerated_trie_root
func ext_blake2_256_enumerated_trie_root(context unsafe.Pointer, valuesData, lensData, lensLen, result int32) {
	log.Trace("[ext_blake2_256_enumerated_trie_root] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	t := &trie.Trie{}

	var i int32
	var pos int32 = 0
	for i = 0; i < lensLen; i++ {
		valueLenBytes := memory[lensData+i*4 : lensData+(i+1)*4]
		valueLen := int32(binary.LittleEndian.Uint32(valueLenBytes))
		value := memory[valuesData+pos : valuesData+pos+valueLen]
		log.Trace("[ext_blake2_256_enumerated_trie_root]", "key", i, "value", fmt.Sprintf("%d", value), "valueLen", valueLen)
		pos += valueLen

		// encode the key
		encodedOutput, err := codec.Encode(big.NewInt(int64(i)))
		if err != nil {
			log.Error("[ext_blake2_256_enumerated_trie_root]", "error", err)
			return
		}
		log.Trace("[ext_blake2_256_enumerated_trie_root]", "key", i, "key value", encodedOutput)
		err = t.Put(encodedOutput, value)
		if err != nil {
			log.Error("[ext_blake2_256_enumerated_trie_root]", "error", err)
			return
		}
	}

	root, err := t.Hash()
	log.Trace("[ext_blake2_256_enumerated_trie_root]", "root hash", fmt.Sprintf("0x%x", root))
	if err != nil {
		log.Error("[ext_blake2_256_enumerated_trie_root]", "error", err)
		return
	}
	copy(memory[result:result+32], root[:])
}

// performs blake2b 256-bit hash of the byte array at memory location `data` with length `length` and saves the
// hash at memory location `out`
//export ext_blake2_256
func ext_blake2_256(context unsafe.Pointer, data, length, out int32) {
	log.Trace("[ext_blake2_256] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	hash, err := common.Blake2bHash(memory[data : data+length])
	if err != nil {
		log.Error("[ext_blake2_256]", "error", err)
		return
	}

	copy(memory[out:out+32], hash[:])
}

//export ext_blake2_128
func ext_blake2_128(context unsafe.Pointer, data, length, out int32) {
	log.Trace("[ext_blake2_128] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	hash, err := common.Blake2b128(memory[data : data+length])
	if err != nil {
		log.Error("[ext_blake2_128]", "error", err)
		return
	}

	copy(memory[out:out+16], hash[:])
}

//export ext_keccak_256
func ext_keccak_256(context unsafe.Pointer, data, length, out int32) {
	log.Trace("[ext_keccak_256] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	hash := common.Keccak256(memory[data : data+length])
	copy(memory[out:out+32], hash[:])
}

//export ext_twox_64
func ext_twox_64(context unsafe.Pointer, data, len, out int32) {
	log.Trace("[ext_twox_64] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	hasher := xxhash.NewS64(0) // create xxHash with 0 seed
	_, err := hasher.Write(memory[data : data+len])
	if err != nil {
		log.Error("[ext_twox_64]", "error", err)
		return
	}

	res := hasher.Sum64()
	hash := make([]byte, 8)
	binary.LittleEndian.PutUint64(hash, uint64(res))
	copy(memory[out:out+8], hash)
}

//export ext_twox_128
func ext_twox_128(context unsafe.Pointer, data, len, out int32) {
	log.Trace("[ext_twox_128] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	// compute xxHash64 twice with seeds 0 and 1 applied on given byte array
	h0 := xxhash.NewS64(0) // create xxHash with 0 seed
	_, err := h0.Write(memory[data : data+len])
	if err != nil {
		log.Error("[ext_twox_128]", "error", err)
		return
	}
	res0 := h0.Sum64()
	hash0 := make([]byte, 8)
	binary.LittleEndian.PutUint64(hash0, uint64(res0))

	h1 := xxhash.NewS64(1) // create xxHash with 1 seed
	_, err = h1.Write(memory[data : data+len])
	if err != nil {
		log.Error("[ext_twox_128]", "error", err)
		return
	}
	res1 := h1.Sum64()
	hash1 := make([]byte, 8)
	binary.LittleEndian.PutUint64(hash1, uint64(res1))

	//concatenated result
	both := append(hash0, hash1...)

	copy(memory[out:out+16], both)
}

//export ext_sr25519_generate
func ext_sr25519_generate(context unsafe.Pointer, idData, seed, seedLen, out int32) {
	log.Trace("[ext_sr25519_generate] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)

	// TODO: key types not yet implemented
	// id := memory[idData:idData+4]

	seedBytes := memory[seed : seed+seedLen]

	kp, err := crypto.NewSr25519KeypairFromSeed(seedBytes)
	if err != nil {
		log.Debug("ext_sr25519_generate cannot generate key", "error", err)
	}

	log.Debug("ext_sr25519_generate", "address", kp.Public().Address())

	runtimeCtx.keystore.Insert(kp)

	copy(memory[out:out+32], kp.Public().Encode())
}

//export ext_ed25519_public_keys
func ext_ed25519_public_keys(context unsafe.Pointer, idData, resultLen int32) int32 {
	log.Trace("[ext_ed25519_public_keys] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)

	keys := runtimeCtx.keystore.Ed25519PublicKeys()
	// TODO: when do deallocate?
	offset, err := runtimeCtx.allocator.Allocate(uint32(len(keys) * 32))
	if err != nil {
		log.Error("[ext_ed25519_public_keys]", "error", err)
		return -1
	}

	for i, key := range keys {
		copy(memory[offset+uint32(i*32):offset+uint32((i+1)*32)], key.Encode())
	}

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(len(keys)))
	copy(memory[resultLen:resultLen+4], buf)
	return int32(offset)
}

//export ext_sr25519_public_keys
func ext_sr25519_public_keys(context unsafe.Pointer, idData, resultLen int32) int32 {
	log.Trace("[ext_sr25519_public_keys] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)

	keys := runtimeCtx.keystore.Sr25519PublicKeys()

	offset, err := runtimeCtx.allocator.Allocate(uint32(len(keys) * 32))
	if err != nil {
		log.Error("[ext_sr25519_public_keys]", "error", err)
		return -1
	}

	for i, key := range keys {
		copy(memory[offset+uint32(i*32):offset+uint32((i+1)*32)], key.Encode())
	}

	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(len(keys)))
	copy(memory[resultLen:resultLen+4], buf)
	return int32(offset)
}

//export ext_ed25519_sign
func ext_ed25519_sign(context unsafe.Pointer, idData, pubkeyData, msgData, msgLen, out int32) int32 {
	log.Debug("[ext_ed25519_sign] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)

	pubkeyBytes := memory[pubkeyData : pubkeyData+32]
	pubkey, err := crypto.NewEd25519PublicKey(pubkeyBytes)
	if err != nil {
		log.Error("[ext_ed25519_sign]", "error", err)
		return 1
	}

	signingKey := runtimeCtx.keystore.GetKeypair(pubkey)
	if signingKey == nil {
		log.Error("[ext_ed25519_sign] could not find key in keystore", "public key", pubkey)
		return 1
	}

	msgLenBytes := memory[msgLen : msgLen+4]
	msgLength := binary.LittleEndian.Uint32(msgLenBytes)
	msg := memory[msgData : msgData+int32(msgLength)]
	sig, err := signingKey.Sign(msg)
	if err != nil {
		log.Error("[ext_ed25519_sign] could not sign message")
		return 1
	}

	copy(memory[out:out+64], sig)
	return 0
}

//export ext_sr25519_sign
func ext_sr25519_sign(context unsafe.Pointer, idData, pubkeyData, msgData, msgLen, out int32) int32 {
	log.Debug("[ext_sr25519_sign] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)

	pubkeyBytes := memory[pubkeyData : pubkeyData+32]
	pubkey, err := crypto.NewSr25519PublicKey(pubkeyBytes)
	if err != nil {
		log.Error("[ext_sr25519_sign]", "error", err)
		return 1
	}

	signingKey := runtimeCtx.keystore.GetKeypair(pubkey)

	if signingKey == nil {
		log.Error("[ext_sr25519_sign] could not find key in keystore", "public key", pubkey)
		return 1
	}

	msgLenBytes := memory[msgLen : msgLen+4]
	msgLength := binary.LittleEndian.Uint32(msgLenBytes)
	msg := memory[msgData : msgData+int32(msgLength)]
	sig, err := signingKey.Sign(msg)
	if err != nil {
		log.Error("[ext_sr25519_sign] could not sign message")
		return 1
	}

	copy(memory[out:out+64], sig)
	return 0
}

//export ext_sr25519_verify
func ext_sr25519_verify(context unsafe.Pointer, msgData, msgLen, sigData, pubkeyData int32) int32 {
	log.Debug("[ext_sr25519_verify] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	msg := memory[msgData : msgData+msgLen]
	sig := memory[sigData : sigData+64]

	pub, err := crypto.NewSr25519PublicKey(memory[pubkeyData : pubkeyData+32])
	if err != nil {
		return 1
	}

	if pub.Verify(msg, sig) {
		return 0
	}

	return 1
}

//export ext_ed25519_generate
func ext_ed25519_generate(context unsafe.Pointer, idData, seed, seedLen, out int32) {
	log.Debug("[ext_ed25519_generate] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	runtimeCtx := instanceContext.Data().(*RuntimeCtx)

	// TODO: key types not yet implemented
	// id := memory[idData:idData+4]

	seedBytes := memory[seed : seed+seedLen]

	kp, err := crypto.NewEd25519KeypairFromSeed(seedBytes)
	if err != nil {
		log.Debug("ext_ed25519_generate cannot generate key", "error", err)
	}

	log.Debug("ext_ed25519_generate", "address", kp.Public().Address())

	runtimeCtx.keystore.Insert(kp)

	copy(memory[out:out+32], kp.Public().Encode())
}

//export ext_ed25519_verify
func ext_ed25519_verify(context unsafe.Pointer, msgData, msgLen, sigData, pubkeyData int32) int32 {
	log.Trace("[ext_ed25519_verify] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	msg := memory[msgData : msgData+msgLen]
	sig := memory[sigData : sigData+64]
	pubkey, err := crypto.NewEd25519PublicKey(memory[pubkeyData : pubkeyData+32])
	if err != nil {
		return 1
	}

	if crypto.Ed25519Verify(pubkey, msg, sig) {
		return 0
	}

	return 1
}

//export ext_secp256k1_ecdsa_recover
func ext_secp256k1_ecdsa_recover(context unsafe.Pointer, msgData, sigData, pubkeyData int32) int32 {
	log.Trace("[ext_secp256k1_ecdsa_recover] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	// msg must be the 32-byte hash of the message to be signed.
	// sig must be a 65-byte compact ECDSA signature containing the
	// recovery id as the last element.
	msg := memory[msgData : msgData+32]
	sig := memory[sigData : sigData+65]

	pub, err := secp256k1.RecoverPubkey(msg, sig)
	if err != nil {
		return 1
	}

	copy(memory[pubkeyData:pubkeyData+65], pub)
	return 0
}

//export ext_is_validator
func ext_is_validator(context unsafe.Pointer) int32 {
	log.Trace("[ext_is_validator] executing...")
	log.Warn("[ext_is_validator] Not yet implemented.")
	return 0
}

//export ext_local_storage_get
func ext_local_storage_get(context unsafe.Pointer, kind, key, keyLen, valueLen int32) int32 {
	log.Trace("[ext_local_storage_get] executing...")
	log.Warn("[ext_local_storage_get] Not yet implemented.")
	return 0
}

//export ext_local_storage_compare_and_set
func ext_local_storage_compare_and_set(context unsafe.Pointer, kind, key, keyLen, oldValue, oldValueLen, newValue, newValueLen int32) int32 {
	log.Trace("[ext_local_storage_compare_and_set] executing...")
	log.Warn("[ext_local_storage_compare_and_set] Not yet implemented.")
	return 0
}

//export ext_network_state
func ext_network_state(context unsafe.Pointer, writtenOut int32) int32 {
	log.Trace("[ext_network_state] executing...")
	log.Warn("[ext_network_state] Not yet implemented.")
	return 0
}

//export ext_submit_transaction
func ext_submit_transaction(context unsafe.Pointer, data, len int32) int32 {
	log.Trace("[ext_submit_transaction] executing...")
	log.Warn("[ext_submit_transaction] Not yet implemented.")
	return 0
}

//export ext_local_storage_set
func ext_local_storage_set(context unsafe.Pointer, kind, key, keyLen, value, valueLen int32) {
	log.Trace("[ext_local_storage_set] executing...")
	log.Warn("[ext_local_storage_set] Not yet implemented.")
}

func registerImports() (*wasm.Imports, error) {
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

	return imports, nil
}
