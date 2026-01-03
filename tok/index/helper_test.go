/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package index

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"testing"
	"unsafe"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	c "github.com/dgraph-io/dgraph/v25/tok/constraints"
	"github.com/stretchr/testify/require"
	"github.com/viterin/vek/vek32"
	"google.golang.org/protobuf/proto"
)

// GenerateMatrix generates a 2D slice of uint64 with varying lengths for each row.
func GenerateMatrix(rows int) ([][]uint64, *pb.SortResult) {
	pbm := &pb.SortResult{}
	matrix := make([][]uint64, rows)
	value := uint64(100)
	for i := range matrix {
		cols := i + 1 // Variable number of columns for each row
		matrix[i] = make([]uint64, cols)
		for j := range matrix[i] {
			matrix[i][j] = value
			value++
		}
		pbm.UidMatrix = append(pbm.UidMatrix, &pb.List{Uids: matrix[i]})
	}
	return matrix, pbm
}

// Encoding and decoding functions
func encodeUint64Matrix(matrix [][]uint64) ([]byte, error) {
	var buf bytes.Buffer

	// Write number of rows
	if err := binary.Write(&buf, binary.LittleEndian, uint64(len(matrix))); err != nil {
		return nil, err
	}

	// Write each row's length and data
	for _, row := range matrix {
		if err := binary.Write(&buf, binary.LittleEndian, uint64(len(row))); err != nil {
			return nil, err
		}
		for _, value := range row {
			if err := binary.Write(&buf, binary.LittleEndian, value); err != nil {
				return nil, err
			}
		}
	}

	return buf.Bytes(), nil
}

func decodeUint64Matrix(data []byte) ([][]uint64, error) {
	buf := bytes.NewReader(data)

	var numRows uint64
	if err := binary.Read(buf, binary.LittleEndian, &numRows); err != nil {
		return nil, err
	}

	matrix := make([][]uint64, numRows)
	for i := range matrix {
		var numCols uint64
		if err := binary.Read(buf, binary.LittleEndian, &numCols); err != nil {
			return nil, err
		}
		matrix[i] = make([]uint64, numCols)
		for j := range matrix[i] {
			if err := binary.Read(buf, binary.LittleEndian, &matrix[i][j]); err != nil {
				return nil, err
			}
		}
	}

	return matrix, nil
}

func encodeUint64MatrixWithGob(matrix [][]uint64) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	if err := enc.Encode(matrix); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func decodeUint64MatrixWithGob(data []byte) ([][]uint64, error) {
	var matrix [][]uint64
	buf := bytes.NewReader(data)
	dec := gob.NewDecoder(buf)

	if err := dec.Decode(&matrix); err != nil {
		return nil, err
	}

	return matrix, nil
}

func encodeUint64MatrixWithJSON(matrix [][]uint64) ([]byte, error) {
	return json.Marshal(matrix)
}

func decodeUint64MatrixWithJSON(data []byte) ([][]uint64, error) {
	var matrix [][]uint64
	if err := json.Unmarshal(data, &matrix); err != nil {
		return nil, err
	}
	return matrix, nil
}

func encodeUint64MatrixUnsafe(matrix [][]uint64) []byte {
	if len(matrix) == 0 {
		return nil
	}

	// Calculate the total size
	var totalSize uint64
	for _, row := range matrix {
		totalSize += uint64(len(row))*uint64(unsafe.Sizeof(uint64(0))) + uint64(unsafe.Sizeof(uint64(0)))
	}
	totalSize += uint64(unsafe.Sizeof(uint64(0)))

	// Create a byte slice with the appropriate size
	data := make([]byte, totalSize)

	offset := 0
	// Write number of rows
	rows := uint64(len(matrix))
	copy(data[offset:offset+8], (*[8]byte)(unsafe.Pointer(&rows))[:])
	offset += 8

	// Write each row's length and data
	for _, row := range matrix {
		rowLen := uint64(len(row))
		copy(data[offset:offset+8], (*[8]byte)(unsafe.Pointer(&rowLen))[:])
		offset += 8
		for i := range row {
			copy(data[offset:offset+8], (*[8]byte)(unsafe.Pointer(&row[i]))[:])
			offset += 8
		}
	}

	return data
}

func decodeUint64MatrixUnsafe(data []byte) ([][]uint64, error) {
	if len(data) == 0 {
		return nil, nil
	}

	offset := 0
	// Read number of rows
	rows := *(*uint64)(unsafe.Pointer(&data[offset]))
	offset += 8

	matrix := make([][]uint64, rows)

	for i := 0; i < int(rows); i++ {
		// Read row length
		rowLen := *(*uint64)(unsafe.Pointer(&data[offset]))
		offset += 8

		matrix[i] = make([]uint64, rowLen)
		for j := 0; j < int(rowLen); j++ {
			matrix[i][j] = *(*uint64)(unsafe.Pointer(&data[offset]))
			offset += 8
		}
	}

	return matrix, nil
}

func encodeUint64MatrixWithProtobuf(protoMatrix *pb.SortResult) ([]byte, error) {
	// Convert the matrix to the protobuf structure
	return proto.Marshal(protoMatrix)
}

func decodeUint64MatrixWithProtobuf(data []byte, protoMatrix *pb.SortResult) error {
	// Unmarshal the protobuf data into the protobuf structure
	return proto.Unmarshal(data, protoMatrix)
}

// Combined benchmark function
func BenchmarkEncodeDecodeUint64Matrix(b *testing.B) {
	matrix, pbm := GenerateMatrix(10)

	b.Run("Binary Encoding/Decoding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data, err := encodeUint64Matrix(matrix)
			if err != nil {
				b.Error(err)
			}
			_, err = decodeUint64Matrix(data)
			if err != nil {
				b.Error(err)
			}
		}
	})

	b.Run("Gob Encoding/Decoding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data, err := encodeUint64MatrixWithGob(matrix)
			if err != nil {
				b.Error(err)
			}
			_, err = decodeUint64MatrixWithGob(data)
			if err != nil {
				b.Error(err)
			}
		}
	})

	b.Run("JSON Encoding/Decoding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data, err := encodeUint64MatrixWithJSON(matrix)
			if err != nil {
				b.Error(err)
			}
			_, err = decodeUint64MatrixWithJSON(data)
			if err != nil {
				b.Error(err)
			}
		}
	})

	b.Run("PB Encoding/Decoding", func(b *testing.B) {
		var pba pb.SortResult
		for i := 0; i < b.N; i++ {
			data, err := encodeUint64MatrixWithProtobuf(pbm)
			if err != nil {
				b.Error(err)
			}

			err = decodeUint64MatrixWithProtobuf(data, &pba)
			if err != nil {
				b.Error(err)
			}
		}
	})

	b.Run("Unsafe Encoding/Decoding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data := encodeUint64MatrixUnsafe(matrix)
			_, err := decodeUint64MatrixUnsafe(data)
			if err != nil {
				b.Error(err)
			}
		}
	})
}

func dotProductT[T c.Float](a, b []T) {
	var product T
	if len(a) != len(b) {
		return
	}
	for i := range a {
		product += a[i] * b[i]
	}
}

func dotProduct(a, b []float32) {
	if len(a) != len(b) {
		return
	}
	sum := int8(0)
	for i := 0; i < len(a); i += 2 {
		sum += *(*int8)(unsafe.Pointer(&a[i]))**(*int8)(unsafe.Pointer(&b[i])) +
			*(*int8)(unsafe.Pointer(&a[i+1]))**(*int8)(unsafe.Pointer(&b[i+1]))
	}
}

func BenchmarkDotProduct(b *testing.B) {
	num := 1500
	data := make([]byte, 64*num)
	_, err := rand.Read(data)
	if err != nil {
		b.Skip()
	}

	b.Run(fmt.Sprintf("vek:size=%d", len(data)),
		func(b *testing.B) {
			temp := make([]float32, num)
			BytesAsFloatArray(data, &temp, 32)
			for k := 0; k < b.N; k++ {
				vek32.Dot(temp, temp)
			}
		})

	b.Run(fmt.Sprintf("dotProduct:size=%d", len(data)),
		func(b *testing.B) {

			temp := make([]float32, num)
			BytesAsFloatArray(data, &temp, 32)
			for k := 0; k < b.N; k++ {
				dotProduct(temp, temp)
			}

		})

	b.Run(fmt.Sprintf("dotProductT:size=%d", len(data)),
		func(b *testing.B) {

			temp := make([]float32, num)
			BytesAsFloatArray(data, &temp, 32)
			for k := 0; k < b.N; k++ {
				dotProductT(temp, temp)
			}
		})
}

func pointerFloatConversion[T c.Float](encoded []byte, retVal *[]T, floatBits int) {
	floatBytes := floatBits / 8

	// Ensure the byte slice length is a multiple of 8 (size of float32)
	if len(encoded)%floatBytes != 0 {
		fmt.Println("Invalid byte slice length")
		return
	}

	// Create a slice header
	*retVal = *(*[]T)(unsafe.Pointer(&encoded))
}

func littleEndianBytesAsFloatArray[T c.Float](encoded []byte, retVal *[]T, floatBits int) {
	// Unfortunately, this is not as simple as casting the result,
	// and it is also not possible to directly use the
	// golang "unsafe" library to directly do the conversion.
	// The machine where this operation gets run might prefer
	// BigEndian/LittleEndian, but the machine that sent it may have
	// preferred the other, and there is no way to tell!
	//
	// The solution below, unfortunately, requires another memory
	// allocation.
	// TODO Potential optimization: If we detect that current machine is
	// using LittleEndian format, there might be a way of making this
	// work with the golang "unsafe" library.
	floatBytes := floatBits / 8

	// Ensure the byte slice length is a multiple of 8 (size of float32)
	if len(encoded)%floatBytes != 0 {
		fmt.Println("Invalid byte slice length")
		return
	}

	*retVal = (*retVal)[:0]
	resultLen := len(encoded) / floatBytes
	if resultLen == 0 {
		return
	}
	for range resultLen {
		// Assume LittleEndian for encoding since this is
		// the assumption elsewhere when reading from client.
		// See dgraph-io/dgo/protos/api.pb.go
		// See also dgraph-io/dgraph/types/conversion.go
		// This also seems to be the preference from many examples
		// I have found via Google search. It's unclear why this
		// should be a preference.
		if retVal == nil {
			retVal = &[]T{}
		}
		*retVal = append(*retVal, BytesToFloat[T](encoded, floatBits))

		encoded = encoded[(floatBytes):]
	}
}

func BenchmarkFloatConversion(b *testing.B) {
	num := 1500
	data := make([]byte, 64*num)
	_, err := rand.Read(data)
	if err != nil {
		b.Skip()
	}

	b.Run(fmt.Sprintf("Current:size=%d", len(data)),
		func(b *testing.B) {
			temp := make([]float32, num)
			for k := 0; k < b.N; k++ {
				BytesAsFloatArray(data, &temp, 64)
			}
		})

	b.Run(fmt.Sprintf("pointerFloat:size=%d", len(data)),
		func(b *testing.B) {
			temp := make([]float32, num)
			for k := 0; k < b.N; k++ {
				pointerFloatConversion(data, &temp, 64)
			}
		})

	b.Run(fmt.Sprintf("littleEndianFloat:size=%d", len(data)),
		func(b *testing.B) {
			temp := make([]float32, num)
			for k := 0; k < b.N; k++ {
				littleEndianBytesAsFloatArray(data, &temp, 64)
			}
		})
}

func TestBytesAsFloatArray(t *testing.T) {
	var out32 []float32
	in32 := []float32{0.1, 1.123456789, 50000.00005}

	inf32 := unsafe.Slice((*byte)(unsafe.Pointer(&in32[0])), len(in32)*int(unsafe.Sizeof(in32[0])))

	BytesAsFloatArray(inf32, &out32, 32)
	require.Equal(t, in32, out32)

	var out64 []float64
	in64 := []float64{0.1, 1.123456789, 50000.00005}

	inf64 := unsafe.Slice((*byte)(unsafe.Pointer(&in64[0])), len(in64)*int(unsafe.Sizeof(in64[0])))

	BytesAsFloatArray(inf64, &out64, 64)
	require.Equal(t, in64, out64)
}
