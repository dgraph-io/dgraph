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

    def __init__(self, int_size, diff_code, in_ptr, out_ptr):
        self.inp = in_ptr
        self.outp = out_ptr
        self.int_size = int_size
        self.diff_code = diff_code

        self.cin = 0
        self.cout = 0

        self.ncopies = 4
        self.copies = []
        self.original = None

    def LOAD(self):
        xmm = self.xmm_reg.pop(0)
        MOVDQU(xmm, [self.inp+self.cin])
        self.cin += 16
        return xmm

    def MASK(self, mask):
        xmm = self.xmm_reg.pop(0)
        xmm_tmp = self.xmm_reg.pop(0)
        rtmp = self.gen_reg.pop(0)

        if self.int_size == 64:
            MOV(rtmp, mask)
            MOVQ(xmm_tmp, rtmp)
            PSHUFD(xmm, xmm_tmp, 0x44)
        elif self.int_size == 32:
            MOV(rtmp, mask)
            MOVQ(xmm_tmp, rtmp)
            PSHUFD(xmm, xmm_tmp, 0x00)

        self.xmm_reg.append(xmm_tmp)
        self.gen_reg.append(rtmp)

        return xmm

    def AND(self, xmm_dst, xmm_src):
        PAND(xmm_dst, xmm_src)
        return xmm_dst

    def OR(self, xmm_dst, xmm_src):
        POR(xmm_dst, xmm_src)
        self.xmm_reg.append(xmm_src)
        return xmm_dst

    def COPY(self, xmm, num):
        if num == 0:
            return [xmm], xmm

        if len(self.copies) == 0:
            num = min(self.ncopies, num)
            self.copies = [self.xmm_reg.pop(0) for _ in range(0, num)]
            for c in self.copies:
                MOVDQA(c, xmm)

            self.copies.insert(0, xmm)
            self.original = self.copies.pop()

        return self.copies, self.original

    def SHR(self, xmms, shift):
        xmm_dst = xmms.pop(0)

        if shift != 0:
            if self.int_size == 64:
                PSRLQ(xmm_dst, shift)
            elif self.int_size == 32:
                PSRLD(xmm_dst, shift)

        return xmm_dst

    def SHL(self, xmms, shift):
        xmm_dst = xmms.pop(0)

        if shift != 0:
            if self.int_size == 64:
                PSLLQ(xmm_dst, shift)
            elif self.int_size == 32:
                PSLLD(xmm_dst, shift)

        return xmm_dst

    def PSUM(self, xmm_dst, xmm_src):
        if self.diff_code:
            if self.int_size == 64:
                PADDQ(xmm_dst, xmm_src)
            elif self.int_size == 32:
                PADDD(xmm_dst, xmm_src)

            self.xmm_reg.append(xmm_src)
            return xmm_dst

        return xmm_src

    def STORE(self, xmm):
        MOVDQU([self.outp+self.cout], xmm)
        self.cout += 16

        if not self.diff_code:
            self.xmm_reg.append(xmm)

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

def unpack(func_name, block_size, bit_size):

    in_ptr = Argument(ptr(uint8_t), name='in')
    out_ptr = Argument(ptr(uint64_t), name='out')
    seed_ptr = Argument(ptr(uint64_t), name='seed')
    int_size = 64

    with Function(func_name, (in_ptr, out_ptr, seed_ptr)):
        MM.CLEAR()

        inp = MM.Register()
        outp = MM.Register()

        LOAD.ARGUMENT(inp, in_ptr)
        LOAD.ARGUMENT(outp, out_ptr)

        seedp = MM.Register()
        prefix_sum = MM.XMMRegister()

        LOAD.ARGUMENT(seedp, seed_ptr)
        MOVDQA(prefix_sum, [seedp])

        i = 0
        mm = MM(int_size, True, inp, outp)
        mask = mm.MASK((1 << bit_size)-1)
        diff_code = True

        in_copies = []
        in_reg = mm.LOAD()
        for k in range(0, (block_size * bit_size)/128):
            while i+bit_size <= int_size:
                num_copy = (int_size-i)/bit_size
                if (int_size-i) % bit_size == 0:
                    num_copy -= 1

                in_copies, in_reg = mm.COPY(in_reg, num_copy)
                out_reg = mm.SHR(in_copies, i)

                if i+bit_size < int_size:
                    out_reg = mm.AND(out_reg, mask)

                prefix_sum = mm.PSUM(prefix_sum, out_reg)
                mm.STORE(prefix_sum)

                i += bit_size

            if i < int_size:
                assert len(in_copies) == 0

                out_reg = mm.SHR([in_reg], i)
                in_reg = mm.LOAD()

                in_copies, in_reg = mm.COPY(in_reg, 1)
                out_reg = mm.OR(out_reg, mm.AND(mm.SHL(in_copies, int_size-i), mask))

                prefix_sum = mm.PSUM(prefix_sum, out_reg)
                mm.STORE(prefix_sum)

                i += bit_size - int_size

            else:
                assert len(in_copies) == 0

                if k != (block_size * bit_size)/128-1:
                    in_reg = mm.LOAD()

                i = 0

        if diff_code:
            MOVDQU([seedp], prefix_sum)

        RETURN()

# Generate code
for bs in range(1, 65):
    unpack('dunpack128_'+str(bs), 128, bs)

for bs in range(1,65):
    unpack('dunpack256_'+str(bs), 256, bs)
