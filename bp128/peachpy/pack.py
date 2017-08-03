from peachpy import *
from peachpy.x86_64 import *

def read(reg, inp, in_offset, seedp):
    if in_offset > 16*63:
        MOVDQU(reg, [seedp])
    else:
        MOVDQU(reg, [inp - in_offset])
    return reg


def pack(func_name, int_size, bit_size):

    in_ptr = Argument(ptr(uint64_t), name='in')
    out_ptr = Argument(ptr(uint8_t), name='out')
    seed_ptr = Argument(ptr(uint64_t), name='seed')

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
        ADD(inp, 16*63)
        ADD(outp, 16*(bit_size - 1))

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
        read(in1, inp, cin, seedp)
        cin += 16
        start = True

        for _ in range(0, bit_size):
            while i <= int_size:
                read(in2,inp, cin, seedp)
                cin += 16
                PSUBQ(in1, in2)
                PSLLQ(in1, int_size-i)
                if start:
                    MOVDQA(out_reg, in1)
                    start = False
                else:
                    POR(out_reg, in1)
                MOVDQA(in1,in2)
                i += bit_size

            if i-bit_size < int_size:
                read(in2, inp, cin, seedp)
                cin += 16
                PSUBQ(in1, in2)
                out_copy = XMMRegister()
                MOVDQU(out_copy,in1)

                PSRLQ(in1, i-int_size)
                POR(out_reg, in1)
                MOVDQU([outp-cout], out_reg)
                cout += 16

                i -= int_size
                PSLLQ(out_copy, int_size-i)
                out_reg = out_copy
                MOVDQA(in1,in2)
                i += bit_size

            else:
                MOVDQU([outp-cout], out_reg)
                cout += 16
                i = bit_size
                start = True

        # Modifies the passed seed slice
        MOVDQU([seedp], last)

        RETURN()

for bs in range(1, 65):
    pack('dpack64_'+str(bs), 64, bs)
