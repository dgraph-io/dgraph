package runtime

import (
	"encoding/binary"
	"fmt"

	common "github.com/ChainSafe/gossamer/common"
	trie "github.com/ChainSafe/gossamer/trie"
	log "github.com/inconshreveable/log15"
	exec "github.com/perlin-network/life/exec"
)

type Resolver struct {
	t *trie.Trie
}

// ResolveFunc resolves the imported functions in the runtime
func (r *Resolver) ResolveFunc(module, field string) exec.FunctionImport {
	switch module {
	case "env":
		switch field {
		case "ext_get_storage_into":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_get_storage_into")
				keyData := int(uint32(vm.GetCurrentFrame().Locals[0]))
				keyLen := int(uint32(vm.GetCurrentFrame().Locals[1]))
				valueData := int(uint32(vm.GetCurrentFrame().Locals[2]))
				valueLen := int(uint32(vm.GetCurrentFrame().Locals[3]))
				valueOffset := int(uint32(vm.GetCurrentFrame().Locals[4]))
				log.Debug("[ext_get_storage_into]", "keyData", keyData, "keyLen", keyLen, "valueData", valueData, "valueLen", valueLen, "valueOffset", valueOffset)

				key := vm.Memory[keyData : keyData+keyLen]
				log.Debug("[ext_get_storage_into]", "key", string(key), "byteskey", key)

				value, err := r.t.Get(key)
				if err != nil {
					log.Error("[ext_get_storage_into]", "error", err)
					return 0
				}

				if valueLen == 0 {
					return 0
				}

				value = value[valueOffset:]
				copy(vm.Memory[valueData:valueData+valueLen], value)

				log.Debug("[ext_get_storage_into]", "value", vm.Memory[valueData:valueData+valueLen])
				ret := int64(binary.LittleEndian.Uint64(common.AppendZeroes(vm.Memory[valueData:valueData+valueLen], 8)))
				log.Debug("[ext_get_storage_into]", "returnvalue", ret)
				return ret
			}
		case "ext_blake2_256":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_blake2_256")
				return 0
			}
		case "ext_blake2_256_enumerated_trie_root":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_blake2_256_enumerated_trie_root")
				return 0
			}
		case "ext_print_utf8":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_print_utf8")
				log.Debug("[ext_print_utf8]", "local[0]", vm.GetCurrentFrame().Locals[0], "local[1]", vm.GetCurrentFrame().Locals[1])
				ptr := int(uint32(vm.GetCurrentFrame().Locals[0]))
				msgLen := int(uint32(vm.GetCurrentFrame().Locals[1]))
				msg := vm.Memory[ptr : ptr+msgLen]
				log.Debug("[ext_print_utf8]", "msg", string(msg))
				return 0
			}
		case "ext_print_num":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_print_num")
				log.Debug("[ext_print_num]", "local[0]", vm.GetCurrentFrame().Locals[0])
				return 0
			}
		case "ext_malloc":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_malloc")
				size := vm.GetCurrentFrame().Locals[0]
				log.Debug("[ext_malloc]", "local[0]", size)
				var offset int64 = 1
				return offset
			}
		case "ext_free":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_free")
				return 1
			}
		case "ext_twox_128":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_twox_128")
				return 0
			}
		case "ext_clear_storage":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_clear_storage")
				return 0
			}
		case "ext_set_storage":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_set_storage")
				return 0
			}
		case "ext_exists_storage":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_exists_storage")
				return 0
			}
		case "ext_sr25519_verify":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_sr25519_verify")
				return 0
			}
		case "ext_ed25519_verify":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_ed25519_verify")
				return 0
			}
		case "ext_storage_root":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_storage_root")
				return 0
			}
		case "ext_storage_changes_root":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_storage_changes_root")
				return 0
			}
		case "ext_print_hex":
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: ext_print_hex")
				return 0
			}
		default:
			return func(vm *exec.VirtualMachine) int64 {
				log.Debug("executing: default")
				return 0
			}
		}
	default:
		panic(fmt.Errorf("unknown module: %s\n", module))
	}
}

func (r *Resolver) ResolveGlobal(module, field string) int64 {
	panic("we're not resolving global variables for now")
}
