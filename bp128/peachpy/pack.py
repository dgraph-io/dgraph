from peachpy import *
from peachpy.x86_64 import *

def add_offset(int_size, array, offset):
    if int_size == 64:
        SHL(offset, 3)
    elif int_size == 32:
        SHL(offset, 2)

    ADD(array, offset)
    return array

class MM:
    gen_reg = [rax, rbx, rcx, rdx,
               rsi, rdi, r8, r9,
               r10, r11, r12, r13,
               r14, r15]
    xmm_reg = [xmm0, xmm1, xmm2, xmm3,
               xmm4, xmm5, xmm6, xmm7,
               xmm8, xmm9, xmm10, xmm11,
               xmm12, xmm13, xmm14, xmm15]

    def __init__(self, int_size, diff_code, in_ptr, out_ptr, seed_ptr):
        self.cout = 0
        self.outp = out_ptr
        self.int_size = int_size
        self.diff_code = diff_code

        self.nbuffer = 4
        self.buffer = []
        self.output_registers = []

        self.in_addr = [[in_ptr-(i*16)] for i in range(0, self.int_size)]
        if diff_code:
            self.in_addr.append([seed_ptr])
        else:
            self.in_addr.append(None)

    def LOAD(self):
        if len(self.buffer) == 0:
            if len(self.xmm_reg) < self.nbuffer:
                self.OR(self.output_registers)

            end = min(self.nbuffer, len(self.in_addr))
            self.buffer = [self.xmm_reg.pop(0) for _ in range(0, end)]
            for i in range(0, end):
                if self.in_addr[0] is not None:
                    MOVDQA(self.buffer[i], self.in_addr.pop(0))

        return self.buffer.pop(0)

    def OR(self, xmm_registers):
        if len(xmm_registers) == 1:
            return xmm_registers[0]

        xmm_in = xmm_registers.pop(0)
        xmm_out = xmm_registers.pop(0)
        POR(xmm_out, xmm_in)

        self.xmm_reg.append(xmm_in)
        xmm_registers.append(xmm_out)

        return self.OR(xmm_registers)

    def SHL(self, xmm, shift):
        if shift != 0:
            if self.int_size == 64:
                PSLLQ(xmm, shift)
            elif self.int_size == 32:
                PSLLD(xmm, shift)

        self.output_registers.append(xmm)
        return self.output_registers

    def SHR(self, xmm, shift):
        if shift != 0:
            if self.int_size == 64:
                PSRLQ(xmm, shift)
            elif self.int_size == 32:
                PSRLD(xmm, shift)

        self.output_registers.append(xmm)
        return self.output_registers

    def STORE(self, xmm):
        MOVDQA([self.outp-self.cout], xmm)
        self.cout += 16

        self.output_registers = []
        self.xmm_reg.append(xmm)

    def COPY(self, xmm):
        if len(self.xmm_reg) == 0:
                self.OR(self.output_registers)

        xmm_copy = self.xmm_reg.pop(0)
        MOVDQA(xmm_copy, xmm)
        return xmm_copy

    def DELTA(self, dst, src):
        if self.diff_code:
            if self.int_size == 64:
                PSUBQ(dst, src)
            elif self.int_size == 32:
                PSUBD(dst, src)

        self.buffer.insert(0, src)
        return dst

    @staticmethod
    def CLEAR():
        MM.gen_reg = [rax, rbx, rcx, rdx,
                      rsi, rdi, r8, r9,
                      r10, r11, r12, r13,
                      r14, r15]
        MM.xmm_reg = [xmm0, xmm1, xmm2, xmm3,
                      xmm4, xmm5, xmm6, xmm7,
                      xmm8, xmm9, xmm10, xmm11,
                      xmm12, xmm13, xmm14, xmm15]

    @staticmethod
    def XMMRegister():
        return MM.xmm_reg.pop(0)

    @staticmethod
    def Register():
        return MM.gen_reg.pop(0)


def pack(func_name, int_size, diff_code, bit_size):

    in_ptr = Argument(ptr(size_t), name='in')
    out_ptr = Argument(ptr(uint8_t), name='out')
    in_offset = Argument(ptrdiff_t, name='offset')
    seed_ptr = Argument(ptr(uint8_t), name='seed')

    with Function(func_name, (in_ptr, out_ptr, in_offset, seed_ptr)):
        MM.CLEAR()

        inp = MM.Register()
        outp = MM.Register()
        inp_offset = MM.Register()

        LOAD.ARGUMENT(inp, in_ptr)
        LOAD.ARGUMENT(outp, out_ptr)
        LOAD.ARGUMENT(inp_offset, in_offset)

        # Move input array to end of block
        inp = add_offset(int_size, inp, inp_offset)
        ADD(inp, 16*(int_size-1))

        # Move output array to end of block
        if bit_size > 1:
            ADD(outp, 16*(bit_size-1))

        last = None
        seedp = None
        if diff_code:
            seedp = MM.Register()
            LOAD.ARGUMENT(seedp, seed_ptr)

            # Store the last vector
            last = MM.XMMRegister()
            MOVDQA(last, [inp])

        i = bit_size
        out_reg = None
        mm = MM(int_size, diff_code, inp, outp, seedp)
        for _ in range(0, bit_size):
            while i <= int_size:
                in1 = mm.LOAD()
                in2 = mm.LOAD()
                out_reg = mm.DELTA(in1, in2)
                out_reg = mm.SHL(out_reg, int_size-i)

                i += bit_size

            if i-bit_size < int_size:
                in1 = mm.LOAD()
                in2 = mm.LOAD()
                out_reg = mm.DELTA(in1, in2)
                out_copy = mm.COPY(out_reg)

                out_reg = mm.SHR(out_reg, i-int_size)
                out_reg = mm.OR(out_reg)
                mm.STORE(out_reg)

                i -= int_size
                out_reg = mm.SHL(out_copy, int_size-i)
                i += bit_size

            else:
                out_reg = mm.OR(out_reg)
                mm.STORE(out_reg)
                i = bit_size

        # Move the last vector to seed
        if diff_code:
            MOVDQA([seedp], last)

        RETURN()

# Generate code
for bs in range(1, 33):
    pack('pack32_'+str(bs), 32, False, bs)
for bs in range(1, 65):
    pack('pack64_'+str(bs), 64, False, bs)

for bs in range(1, 33):
    pack('dpack32_'+str(bs), 32, True, bs)
for bs in range(1, 65):
    pack('dpack64_'+str(bs), 64, True, bs)
