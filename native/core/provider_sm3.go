package core

import "encoding/binary"

// SM3 国标摘要（GB/T 32905-2016）。
//
// 这里自带实现而不是引第三方模块：调用方只需要一次 32 字节摘要，
// 引一个整包依赖（如 gmsm）代价过高。

var hongguoSM3IV = [8]uint32{
	0x7380166f, 0x4914b2b9, 0x172442d7, 0xda8a0600,
	0xa96f30bc, 0x163138aa, 0xe38dee4d, 0xb0fb0e4e,
}

func hongguoSM3P0(x uint32) uint32 { return x ^ rotl32(x, 9) ^ rotl32(x, 17) }
func hongguoSM3P1(x uint32) uint32 { return x ^ rotl32(x, 15) ^ rotl32(x, 23) }

func rotl32(x uint32, n uint) uint32 { return x<<(n&31) | x>>(32-(n&31)) }

func hongguoSM3Block(state *[8]uint32, block []byte) {
	var w [68]uint32
	var w1 [64]uint32
	for i := 0; i < 16; i++ {
		w[i] = binary.BigEndian.Uint32(block[i*4:])
	}
	for i := 16; i < 68; i++ {
		w[i] = hongguoSM3P1(w[i-16]^w[i-9]^rotl32(w[i-3], 15)) ^ rotl32(w[i-13], 7) ^ w[i-6]
	}
	for i := 0; i < 64; i++ {
		w1[i] = w[i] ^ w[i+4]
	}
	a, b, c, d := state[0], state[1], state[2], state[3]
	e, f, g, h := state[4], state[5], state[6], state[7]
	for j := 0; j < 64; j++ {
		var ff, gg, tj uint32
		if j < 16 {
			ff = a ^ b ^ c
			gg = e ^ f ^ g
			tj = 0x79cc4519
		} else {
			ff = (a & b) | (a & c) | (b & c)
			gg = (e & f) | (^e & g)
			tj = 0x7a879d8a
		}
		ss1 := rotl32(rotl32(a, 12)+e+rotl32(tj, uint(j)), 7)
		ss2 := ss1 ^ rotl32(a, 12)
		tt1 := ff + d + ss2 + w1[j]
		tt2 := gg + h + ss1 + w[j]
		d = c
		c = rotl32(b, 9)
		b = a
		a = tt1
		h = g
		g = rotl32(f, 19)
		f = e
		e = hongguoSM3P0(tt2)
	}
	state[0] ^= a
	state[1] ^= b
	state[2] ^= c
	state[3] ^= d
	state[4] ^= e
	state[5] ^= f
	state[6] ^= g
	state[7] ^= h
}

// hongguoSM3Sum 计算 SM3 摘要。
func hongguoSM3Sum(data []byte) []byte {
	state := hongguoSM3IV
	length := len(data)
	bitLength := uint64(length) * 8

	full := length / 64 * 64
	for i := 0; i < full; i += 64 {
		hongguoSM3Block(&state, data[i:i+64])
	}

	tail := make([]byte, 0, 128)
	tail = append(tail, data[full:]...)
	tail = append(tail, 0x80)
	for len(tail)%64 != 56 {
		tail = append(tail, 0)
	}
	var lengthBytes [8]byte
	binary.BigEndian.PutUint64(lengthBytes[:], bitLength)
	tail = append(tail, lengthBytes[:]...)
	for i := 0; i < len(tail); i += 64 {
		hongguoSM3Block(&state, tail[i:i+64])
	}

	out := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(out[i*4:], state[i])
	}
	return out
}
