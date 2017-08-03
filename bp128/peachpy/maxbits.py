from peachpy import *
from peachpy.x86_64 import *

class MM:
    gen_reg = [rax, rbx, rcx, rdx,
               rsi, rdi, r8, r9,
               r10, r11, r12, r13,
               r14, r15]
    xmm_reg = [xmm0, xmm1, xmm2, xmm3,
               xmm4, xmm5, xmm6, xmm7,
               xmm8, xmm9, xmm10, xmm11,
               xmm12, xmm13, xmm14, xmm15]

    def __init__(self, in_ptr):
        self.inp = in_ptr

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
                MOVDQU(self.buffer[i], [self.inp-self.cin])
                self.cin += 16

        return self.buffer.pop(0)

    def DELTA(self, dst, src):
        PSUBQ(dst, src)

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

        PSHUFD(tmp, accumulator, 0x0E)
        POR(accumulator, tmp)
        MOVQ(bits, accumulator)

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

def max_bits(func_name, block_size):

    array_ptr = Argument(ptr(uint64_t), name='in')
    seed_ptr = Argument(ptr(uint64_t), name='seed')

    with Function(func_name, (array_ptr, seed_ptr), uint8_t):
        MM.CLEAR()

        inp = MM.Register()

        LOAD.ARGUMENT(inp, array_ptr)
        ADD(inp, (block_size*64)/8 - 16)

        # Initialize seed
        seedp = MM.Register()
        LOAD.ARGUMENT(seedp, seed_ptr)

        # Store the last vector
        last = MM.XMMRegister()
        MOVDQU(last, [inp])

        # Iterate from the end to
        # the beginning of the block
        mm = MM(inp)
        for _ in range(0, (block_size * 64)/128 -1):
            in1 = mm.LOAD()
            in2 = mm.LOAD()
            mm.ACCUMULATE(mm.DELTA(in1, in2))

        # process the last vector
        out = mm.LOAD()
        accumulators = mm.ACCUMULATE(mm.DELTA(out, [seedp]))
        result = mm.MAXBITS(accumulators)

        RETURN(result.as_low_byte)

max_bits('dmaxBits128', 128)
max_bits('dmaxBits256', 256)
