---
layout: default
title: SCALE Examples
permalink: /scale-examples/
---

A byte array `A` is defined as a set of bytes `(b1,...,bn)` st. `n < 2^536`.

Let `LS(x, n)` be the least significant `n` bits of `x`

We encode a mode as the least significant two bits of the first byte of each encoding.

We use `|` to mean append. `LE` means little endian format

### n < 2^6; mode = 0
`Enc(A) := l_1 | b_1 |...| b_n when n < 2^6`

`l_1` is the encoding of `n`. Since `n < 2^6` we know `n` is at most 6 bits. We take these 6 bits and append `00` to indicate the 'mode' we are using.

eg. `n = 0x4 = 00000100`

`=> LS(n, 6) = 000100`

`=> Enc(A) := 000100| 00 | b_1 | ... | b_4`      (ie. `LS(n,6)` | mode | bytes)


### 2^6 <= n < 2^14; mode = 1
`Enc(A) := i_1 | i_2 | b_1 |...| b_n` when `2^6 <= n < 2^14`

`i_1,i_2` are the encoding of `n`. Since `2^6 <= n < 2^14` we know `n` is between 7 and 14 bits. We truncate `n` to 14 bits (removing leading zeros or padding to 14 bits) and append `01` to indicate the 'mode'.

eg. `n = 0x1000 = 00010000,00000000` 

`=> LS(n, 14) = 010000,00000000`

`=> 01000000,00000001` # Append mode bits, will store in LE in next step

`=> Enc(A) := 00000001 | 01000000 | b_1 | ... | b_4096` (ie. `LS(n,14)` | mode | bytes)

### 2^14 <= n < 2^30; mode = 2
`Enc(A) := i_1 | i_2 | i_3 | i_4 | b_1 |...| b_n` when `2^14 <= n < 2^30`

This is the same as the previous case, but now `n` occupies 30 bits and the mode is `2`.

eg. `n = 0x240FF80 = 00000010,01000000,11111111,10000000` 

`=> LS(n, 30) = 000010,01000000,11111111,10000000`

`=> 00001001,00000011,11111110,00000010`  # Append mode bits, will store in LE in next step

`=> Enc(A) := 00000010 | 11111110 | 00000011 | 00001001 | b_1 | ... | b_37814144` (ie. `LS(n,30)` | mode | bytes)

### 2^30 <= n < 2^536
`Enc(A) := k_1 | k_2 | k_3 | k_4 | k_5 | b_1 |...| b_n` when `2^30 <= n < 2^536`

In this case we define `m` to be the number of bytes used to store `n`. We then assign `k_1 := m - 4` to indicate how many bytes to read for the size of `n`. 

eg. `n = 0x100000000 = 00000001 | 0...0 | 0...0 | 0...0 | 0...0`

` => m = 5 `

`=> k_1 = m - 4 = 1`

`=> 000001 | 11 | 0...0 | 0...0 | 0...0 | 0...0 | b_1 | ... | b_4294967296` (ie. `m - 4` | mode | n | bytes)
