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
// extern void ext_twox_128(void *context, int32_t data, int32_t len, int32_t out);
// extern int32_t ext_get_allocated_storage(void *context, int32_t keyData, int32_t keyLen, int32_t writtenOut);
// extern void ext_storage_root(void *context, int32_t resultPtr);
// extern int32_t ext_storage_changes_root(void *context, int32_t a, int32_t b, int32_t c);
// extern void ext_clear_prefix(void *context, int32_t prefixData, int32_t prefixLen);
// extern int32_t ext_sr25519_verify(void *context, int32_t msgData, int32_t msgLen, int32_t sigData, int32_t pubkeyData);
// extern int32_t ext_ed25519_verify(void *context, int32_t msgData, int32_t msgLen, int32_t sigData, int32_t pubkeyData);
// extern void ext_blake2_256_enumerated_trie_root(void *context, int32_t valuesData, int32_t lensData, int32_t lensLen, int32_t result);
// extern void ext_print_num(void *context, int64_t data);
import "C"

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	scale "github.com/ChainSafe/gossamer/codec"
	common "github.com/ChainSafe/gossamer/common"
	trie "github.com/ChainSafe/gossamer/trie"
	log "github.com/ChainSafe/log15"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
	ed25519 "golang.org/x/crypto/ed25519"
)

//export ext_print_num
func ext_print_num(context unsafe.Pointer, data C.int64_t) {
	log.Debug("[ext_print_num] executing...")
	log.Debug("[ext_print_num]", "message", fmt.Sprintf("%d", data))
}

//export ext_malloc
func ext_malloc(context unsafe.Pointer, size C.int32_t) C.int32_t {
	log.Debug("[ext_malloc] executing...")
	log.Debug("[ext_malloc]", "size", size)
	return 1
}

//export ext_free
func ext_free(context unsafe.Pointer, addr C.int32_t) {
	log.Debug("[ext_free] executing...")
	log.Debug("[ext_free]", "addr", addr)
}

// prints string located in memory at location `offset` with length `size`
//export ext_print_utf8
func ext_print_utf8(context unsafe.Pointer, utf8_data, utf8_len int32) {
	log.Debug("[ext_print_utf8] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	log.Debug("[ext_print_utf8]", "message", fmt.Sprintf("%s", memory[utf8_data:utf8_data+utf8_len]))
}

// prints hex formatted bytes located in memory at location `offset` with length `size`
//export ext_print_hex
func ext_print_hex(context unsafe.Pointer, offset, size int32) {
	log.Debug("[ext_print_hex] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	log.Debug("[ext_print_hex]", "message", fmt.Sprintf("%x", memory[offset:offset+size]))
}

// gets the key stored at memory location `keyData` with length `keyLen` and stores the value in memory at
// location `valueData`. the value can have up to value `valueLen` and the returned value starts at value[valueOffset:]
//export ext_get_storage_into
func ext_get_storage_into(context unsafe.Pointer, keyData, keyLen, valueData, valueLen, valueOffset int32) int32 {
	log.Debug("[ext_get_storage_into] executing...")

	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	t := (*trie.Trie)(instanceContext.Data())

	key := memory[keyData : keyData+keyLen]
	val, err := t.Get(key)
	if err != nil || val == nil {
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
	log.Debug("[ext_set_storage] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	t := (*trie.Trie)(instanceContext.Data())

	key := memory[keyData : keyData+keyLen]
	val := memory[valueData : valueData+valueLen]
	log.Debug("[ext_set_storage]", "key", key, "val", val)
	err := t.Put(key, val)
	if err != nil {
		log.Error("[ext_set_storage]", "error", err)
	}
}

// returns the trie root in the memory location `resultPtr`
//export ext_storage_root
func ext_storage_root(context unsafe.Pointer, resultPtr int32) {
	log.Debug("[ext_storage_root] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	t := (*trie.Trie)(instanceContext.Data())

	root, err := t.Hash()
	if err != nil {
		log.Error("[ext_storage_root]", "error", err)
	}

	copy(memory[resultPtr:resultPtr+32], root[:])
}

//export ext_storage_changes_root
func ext_storage_changes_root(context unsafe.Pointer, a, b, c int32) int32 {
	log.Debug("[ext_storage_changes_root] executing...")
	return 0
}

// gets value stored at key at memory location `keyData` with length `keyLen` and returns the location
// in memory where it's stored and stores its length in `writtenOut`
//export ext_get_allocated_storage
func ext_get_allocated_storage(context unsafe.Pointer, keyData, keyLen, writtenOut int32) int32 {
	log.Debug("[ext_get_allocated_storage] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	t := (*trie.Trie)(instanceContext.Data())

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
	log.Debug("[ext_sr25519_verify] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	t := (*trie.Trie)(instanceContext.Data())

	key := memory[keyData : keyData+keyLen]
	err := t.Delete(key)
	if err != nil {
		log.Error("[ext_storage_root]", "error", err)
	}
}

// deletes all entries in the trie that have a key beginning with the prefix stored at `prefixData`
//export ext_clear_prefix
func ext_clear_prefix(context unsafe.Pointer, prefixData, prefixLen int32) {
	log.Debug("[ext_clear_prefix] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	t := (*trie.Trie)(instanceContext.Data())

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
	log.Debug("[ext_blake2_256_enumerated_trie_root] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	t := &trie.Trie{}

	var i int32
	var pos int32 = 0
	for i = 0; i < lensLen; i++ {
		valueLenBytes := memory[lensData+i*4 : lensData+(i+1)*4]
		valueLen := int32(binary.LittleEndian.Uint32(valueLenBytes))
		value := memory[valuesData+pos : valuesData+pos+valueLen]
		log.Debug("[ext_blake2_256_enumerated_trie_root]", "key", i, "value", fmt.Sprintf("%x", value), "valueLen", valueLen)
		pos += valueLen

		err := t.Put([]byte{byte(i)}, value)
		if err != nil {
			log.Error("[ext_blake2_256_enumerated_trie_root]", "error", err)
		}
	}

	root, err := t.Hash()
	if err != nil {
		log.Error("[ext_blake2_256_enumerated_trie_root]", "error", err)
	}

	copy(memory[result:result+32], root[:])
}

// performs blake2b 256-bit hash of the byte array at memory location `data` with length `length` and saves the
// hash at memory location `out`
//export ext_blake2_256
func ext_blake2_256(context unsafe.Pointer, data, length, out int32) {
	log.Debug("[ext_blake2_256] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()
	hash, err := common.Blake2bHash(memory[data : data+length])
	if err != nil {
		log.Error("[ext_blake2_256]", "error", err)
	}

	copy(memory[out:out+32], hash[:])
}

//export ext_twox_128
func ext_twox_128(context unsafe.Pointer, data, len, out int32) {
	log.Debug("[ext_twox_128] executing...")
}

//export ext_sr25519_verify
func ext_sr25519_verify(context unsafe.Pointer, msgData, msgLen, sigData, pubkeyData int32) int32 {
	log.Debug("[ext_sr25519_verify] executing...")
	return 0
}

//export ext_ed25519_verify
func ext_ed25519_verify(context unsafe.Pointer, msgData, msgLen, sigData, pubkeyData int32) int32 {
	log.Debug("[ext_ed25519_verify] executing...")
	instanceContext := wasm.IntoInstanceContext(context)
	memory := instanceContext.Memory().Data()

	msg := memory[msgData : msgData+msgLen]
	sig := memory[sigData : sigData+64]
	pubkey := ed25519.PublicKey(memory[pubkeyData : pubkeyData+32])

	if ed25519.Verify(pubkey, msg, sig) {
		return 0
	}

	return 1
}

type Runtime struct {
	vm   wasm.Instance
	trie *trie.Trie
}

func NewRuntime(fp string, t *trie.Trie) (*Runtime, error) {
	// Reads the WebAssembly module as bytes.
	bytes, err := wasm.ReadBytes(fp)
	if err != nil {
		return nil, err
	}

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

	// Instantiates the WebAssembly module.
	instance, err := wasm.NewInstanceWithImports(bytes, imports)
	if err != nil {
		return nil, err
	}

	data := unsafe.Pointer(t)
	instance.SetContextData(data)

	return &Runtime{
		vm:   instance,
		trie: t,
	}, nil
}

func (r *Runtime) Stop() {
	r.vm.Close()
}

func (r *Runtime) Exec(function string, data, len int32) ([]byte, error) {
	runtimeFunc, ok := r.vm.Exports[function]
	if !ok {
		return nil, errors.New("could not find exported function")
	}

	res, err := runtimeFunc(data, len)
	if err != nil {
		return nil, err
	}
	resi := res.ToI64()

	length := int32(resi >> 32)
	offset := int32(resi)
	fmt.Printf("offset %d length %d\n", offset, length)
	mem := r.vm.Memory.Data()
	rawdata := make([]byte, length)
	copy(rawdata, mem[offset:offset+length])

	return rawdata, err
}

func decodeToInterface(in []byte, t interface{}) (interface{}, error) {
	buf := &bytes.Buffer{}
	sd := scale.Decoder{Reader: buf}
	_, err := buf.Write(in)
	if err != nil {
		return nil, err
	}

	output, err := sd.Decode(t)
	return output, err
}
