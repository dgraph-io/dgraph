// +build ignore

package main

import (
	"fmt"
	"math/bits"
	"math/rand"
	"time"

	"github.com/dgryski/go-groupvarint"
)

func generateBitsUsed() {

	fmt.Println("package groupvarint")
	fmt.Println()
	fmt.Println("var BytesUsed = []int{")

	for i := 0; i < 256; i++ {
		b := byte(i)

		used := 1

		for j := 0; j < 4; j++ {
			used += int(b&3) + 1
			b >>= 2
		}

		if i == 0 || (i > 1 && i%16 == 1) {
			fmt.Printf("\t")
		}
		fmt.Printf("%d, ", used)
		if i > 0 && i%16 == 0 {
			fmt.Printf("\n")
		}
	}

	fmt.Println()
	fmt.Println("}")
}

func generateSSEMasks() {

	fmt.Println("// +build amd64 !noasm")
	fmt.Println()
	fmt.Println("package groupvarint")
	fmt.Println()
	fmt.Println("var sseMasks = []uint64{")

	for i := uint(0); i < 256; i++ {

		var offs uint32
		var vals [4]uint32

		for j := uint(0); j < 4; j++ {
			d := 1 + ((i >> (2 * j)) & 3)

			for k := uint(0); k < d; k++ {
				vals[j] |= offs << (8 * k)
				offs++
			}

			for k := d; k < 4; k++ {
				vals[j] |= 0xff << (8 * k)
			}
		}

		fmt.Printf("\t0x%08x%08x, 0x%08x%08x,\n", vals[1], vals[0], vals[3], vals[2])
	}

	fmt.Println("}")
}

func makeInput(n int) []uint32 {
	rand.Seed(0)

	var input []uint32

	for i := 0; i < n; i++ {

		for j := 0; j < 4; j++ {

			b := uint32(rand.Int31())

			size := bits.LeadingZeros32(b)

			var u32 uint32

			switch size {
			// case 0: none, because b > 0
			case 1:
				u32 = uint32(rand.Intn(1 << 8))
			case 2:
				u32 = 1<<8 + uint32(rand.Intn((1<<16)-(1<<8)))
			case 3:
				u32 = 1<<16 + uint32(rand.Intn((1<<24)-(1<<16)))
			default:
				u32 = 1<<24 + uint32(rand.Intn((1<<32)-(1<<24)))
			}

			input = append(input, u32)
		}

	}

	return input
}

func encodeGroupVarint(input []uint32) []byte {

	var r []byte

	var padding int
	for len(input) > 0 {
		var dst [17]byte

		d := groupvarint.Encode4(dst[:], input)

		padding = 17 - len(d)

		r = append(r, d...)
		fmt.Println(len(r))
		for _, v := range r {
			fmt.Printf("% 08b", v)
		}
		fmt.Println()
		input = input[4:]
	}

	// must be able to load 17 bytes from start of final block
	for i := 0; i < padding; i++ {
		r = append(r, 0)
	}

	return r
}

func decodeTesting() {

	nums := makeInput(1)
	fmt.Println(nums)
	encSTime := time.Now()
	input := encodeGroupVarint(nums)
	//fmt.Println(input)
	fmt.Println("Encoding time", time.Since(encSTime))
	tmpUids := make([]uint32, 0)
	for i := 0; i < 1; i++ {
		decSTime := time.Now()
		src := input
		for len(src) > 17 {
			var dst [4]uint32
			groupvarint.Decode4(dst[:], src)
			tmpUids = append(tmpUids, dst[0], dst[1], dst[2], dst[3])
			src = src[groupvarint.BytesUsed[src[0]]:]
		}
		fmt.Println("Decoding time", time.Since(decSTime))
	}
	for k, v := range tmpUids {
		// fmt.Printf("%d %d\n", nums[k], v)
		if nums[k] != v {
			fmt.Printf("number mismatch: %d %d\n", nums[k], v)
		}
	}
	// fmt.Println("Hello")
}

func main() {

	//table := flag.String("table", "", "which table to generate: bytesused,ssemasks")
	//
	//flag.Parse()
	//
	//switch *table {
	//case "bytesused":
	//	generateBitsUsed()
	//case "ssemasks":
	//	generateSSEMasks()
	//default:
	//	log.Fatalf("unknown table: %q", *table)
	//}
	decodeTesting()
}
