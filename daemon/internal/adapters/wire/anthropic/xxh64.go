// xxh64 is the checksum Claude Code stamps into a subscription billing header.
package anthropic

import "encoding/binary"

const (
	xxhPrime1 = 0x9E3779B185EBCA87
	xxhPrime2 = 0xC2B2AE3D27D4EB4F
	xxhPrime3 = 0x165667B19E3779F9
	xxhPrime4 = 0x85EBCA77C2B2AE63
	xxhPrime5 = 0x27D4EB2F165667C5
)

func xxh64(input []byte, seed uint64) uint64 {
	n := len(input)
	var acc uint64
	if n >= 32 {
		acc1 := seed + xxhPrime1 + xxhPrime2
		acc2 := seed + xxhPrime2
		acc3 := seed
		acc4 := seed - xxhPrime1
		for len(input) >= 32 {
			acc1 = xxhRound(acc1, binary.LittleEndian.Uint64(input[0:8]))
			acc2 = xxhRound(acc2, binary.LittleEndian.Uint64(input[8:16]))
			acc3 = xxhRound(acc3, binary.LittleEndian.Uint64(input[16:24]))
			acc4 = xxhRound(acc4, binary.LittleEndian.Uint64(input[24:32]))
			input = input[32:]
		}
		acc = rotl(acc1, 1) + rotl(acc2, 7) + rotl(acc3, 12) + rotl(acc4, 18)
		acc = xxhMerge(acc, acc1)
		acc = xxhMerge(acc, acc2)
		acc = xxhMerge(acc, acc3)
		acc = xxhMerge(acc, acc4)
	} else {
		acc = seed + xxhPrime5
	}
	acc += uint64(n)
	return xxhTail(acc, input)
}

func xxhRound(acc, lane uint64) uint64 {
	acc += lane * xxhPrime2
	acc = rotl(acc, 31)
	acc *= xxhPrime1
	return acc
}

func xxhMerge(acc, lane uint64) uint64 {
	acc ^= xxhRound(0, lane)
	acc = acc*xxhPrime1 + xxhPrime4
	return acc
}

func xxhTail(acc uint64, input []byte) uint64 {
	for len(input) >= 8 {
		acc ^= xxhRound(0, binary.LittleEndian.Uint64(input[:8]))
		acc = rotl(acc, 27)*xxhPrime1 + xxhPrime4
		input = input[8:]
	}
	if len(input) >= 4 {
		acc ^= uint64(binary.LittleEndian.Uint32(input[:4])) * xxhPrime1
		acc = rotl(acc, 23)*xxhPrime2 + xxhPrime3
		input = input[4:]
	}
	for _, b := range input {
		acc ^= uint64(b) * xxhPrime5
		acc = rotl(acc, 11) * xxhPrime1
	}
	acc ^= acc >> 33
	acc *= xxhPrime2
	acc ^= acc >> 29
	acc *= xxhPrime3
	acc ^= acc >> 32
	return acc
}

func rotl(value uint64, shift int) uint64 {
	return (value << shift) | (value >> (64 - shift))
}
