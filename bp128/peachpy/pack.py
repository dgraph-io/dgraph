from peachpy import *
from peachpy.x86_64 import *

def read(reg, inp, in_offset, seedp, block_size):
    if in_offset > (block_size*64)/8 - 16:
        MOVDQU(reg, [seedp])
    else:
        MOVDQU(reg, [inp - in_offset])
    return reg


def pack(func_name, block_size, bit_size):

    in_ptr = Argument(ptr(uint64_t), name='in')
    out_ptr = Argument(ptr(uint8_t), name='out')
    seed_ptr = Argument(ptr(uint64_t), name='seed')
    int_size = 64

    with Function(func_name, (in_ptr, out_ptr, seed_ptr)):
        inp = GeneralPurposeRegister64()
        outp = GeneralPurposeRegister64()
        seedp = GeneralPurposeRegister64()

        LOAD.ARGUMENT(inp, in_ptr)
        LOAD.ARGUMENT(outp, out_ptr)
        LOAD.ARGUMENT(seedp, seed_ptr)

        # Move input array to the end of the block
        # We can do inplace delta calculations with copying if we
        # iterate from back,i.e. we point to the last 16bytes(128bits)
        ADD(inp, (block_size * int_size)/8 - 16)
        ADD(outp, (block_size*bit_size)/8 - 16)

        # Store the last vector
        last = XMMRegister()
        # MOV unaligned
        MOVDQU(last, [inp])

        cin = 0
        cout = 0

        i = bit_size
        out_reg = XMMRegister()
        in1 = XMMRegister()
        in2  = XMMRegister()
        read(in1, inp, cin, seedp, block_size)
        cin += 16
        in_regs = [in1, in2]
        start = True

        for _ in range(0, (block_size* bit_size)/128):
            while i <= int_size:
                # Read the next 16 bytes into register
                read(in_regs[1],inp,cin,seedp, block_size)
                cin += 16
                # Find the delta
                PSUBQ(in_regs[0],in_regs[1])
                # Left shift to bit pack
                PSLLQ(in_regs[0], int_size-i)
                if start:
                    MOVDQA(out_reg, in_regs[0])
                    start = False
                else:
                    # OR with the previous output register
                    POR(out_reg, in_regs[0])
                i += bit_size
                in_regs.reverse()

            if i-bit_size < int_size:
                # Read the next 16 bytes into register
                read(in_regs[1], inp, cin, seedp, block_size)
                cin += 16
                # Find the delta
                PSUBQ(in_regs[0],in_regs[1])
                # This integer would be split across two 128 bit
                # registers, so we make a copy as we need to do 
                # both right and left shifting and simd instructions
                # modify the data in place
                out_copy = XMMRegister()
                MOVDQU(out_copy,in_regs[0])

                # Or the MSB Bits into the output register and write it
                # out
                PSRLQ(in_regs[0], i-int_size)
                POR(out_reg, in_regs[0])
                MOVDQU([outp-cout], out_reg)
                cout += 16

                i -= int_size
                # Write the remaining bits(LSB) into the next 128 bit register
                PSLLQ(out_copy, int_size-i)
                out_reg = out_copy
                i += bit_size
                in_regs.reverse()

            else:
                # Write out the output register
                MOVDQU([outp-cout], out_reg)
                cout += 16
                i = bit_size
                start = True

        # Modifies the passed seed slice
        MOVDQU([seedp], last)

        RETURN()

for bs in range(1, 65):
    pack('dpack128_'+str(bs), 128, bs)

for bs in range(1,65):
    pack('dpack256_'+str(bs), 256, bs)
