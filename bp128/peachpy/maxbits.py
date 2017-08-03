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

    def __init__(self, int_size, diff_code, in_ptr):
        self.inp = in_ptr
        self.int_size = int_size
        self.diff_code = diff_code

        self.cin = 0
        self.buffer = []
        self.nbuffer = 4
        self.accumulators = [self.xmm_reg.pop(0) for _ in range(0, self.nbuffer)]
        for acc in self.accumulators:
            PXOR(acc, acc)

    def LOAD(self):
        if len(self.buffer) == 0:
            self.buffer = [self.xmm_reg.pop(0) for _ in range(0, self.nbuffer)]
            for i in range(0, self.nbuffer):
                MOVDQA(self.buffer[i], [self.inp-self.cin])
                self.cin += 16

        return self.buffer.pop(0)

    def DELTA(self, dst, src):
        if self.diff_code:
            if self.int_size == 64:
                PSUBQ(dst, src)
            elif self.int_size == 32:
                PSUBD(dst, src)

        self.buffer.insert(0, src)
        return dst

    def ACCUMULATE(self, xmm):
        acc = self.accumulators.pop(0)
        POR(acc, xmm)

        self.xmm_reg.append(xmm)
        self.accumulators.append(acc)

        return self.accumulators

    def OR(self, xmm_registers):
        if len(xmm_registers) == 1:
            return xmm_registers[0]

        xmm_in = xmm_registers.pop(0)
        xmm_out = xmm_registers.pop(0)
        POR(xmm_out, xmm_in)

        self.xmm_reg.append(xmm_in)
        xmm_registers.append(xmm_out)

        return self.OR(xmm_registers)

    def MAXBITS(self, accumulators):
        bits = self.Register()
        tmp = self.XMMRegister()
        accumulator = self.OR(accumulators)

        if self.int_size == 64:
            PSHUFD(tmp, accumulator, 0x0E)
            POR(accumulator, tmp)
            MOVQ(bits, accumulator)
        elif self.int_size == 32:
            PSHUFD(tmp, accumulator, 0x31)
            POR(accumulator, tmp)
            PSHUFD(tmp, accumulator, 0x02)
            POR(accumulator, tmp)
            MOVD(bits.as_dword, accumulator)

        result = self.Register()
        BSR(result, bits)
        ADD(result, 1)

        # Return zero if bits is zero
        TEST(bits, bits)
        CMOVZ(result, bits)

        return result

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

def max_bits128(func_name, int_size, diff_code):

    array_ptr = Argument(ptr(size_t), name='in')
    offset = Argument(ptrdiff_t, name='offset')
    seed_ptr = Argument(ptr(uint8_t), name='seed')

    with Function(func_name, (array_ptr, offset, seed_ptr), uint8_t):
        MM.CLEAR()

        inp = MM.Register()
        inp_offset = MM.Register()

        LOAD.ARGUMENT(inp, array_ptr)
        LOAD.ARGUMENT(inp_offset, offset)

        # Move array to end of block
        inp = add_offset(int_size, inp, inp_offset)
        ADD(inp, 16*(int_size-1))

        last = None
        seedp = None
        if diff_code:
            # Initialize seed
            seedp = MM.Register()
            LOAD.ARGUMENT(seedp, seed_ptr)

            # Store the last vector
            last = MM.XMMRegister()
            MOVDQA(last, [inp])

        # Iterate from the end to
        # the beginning of the block
        mm = MM(int_size, diff_code, inp)
        for _ in range(0, int_size-1):
            in1 = mm.LOAD()
            in2 = mm.LOAD()
            mm.ACCUMULATE(mm.DELTA(in1, in2))

        # process the last vector
        out = mm.LOAD()
        accumulators = mm.ACCUMULATE(mm.DELTA(out, [seedp]))
        result = mm.MAXBITS(accumulators)

        # Move the last vector to seed
        if diff_code:
            MOVDQA([seedp], last)

        RETURN(result.as_low_byte)

# Generate code
max_bits128('maxBits128_64', 64, False)
max_bits128('maxBits128_32', 32, False)

max_bits128('dmaxBits128_64', 64, True)
max_bits128('dmaxBits128_32', 32, True)
