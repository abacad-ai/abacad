package rfb

import "crypto/des"

// Response computes the 16-byte VNC authentication response for a challenge.
// Classic VNC auth uses DES-ECB with the password as the key, with the bits of
// each password byte reversed. The password is truncated or zero-padded to the
// protocol's fixed 8 bytes.
func Response(challenge []byte, password string) []byte {
	if len(challenge) != 16 {
		return nil
	}
	key := make([]byte, 8)
	copy(key, []byte(password))
	for i := range key {
		key[i] = reverseBits(key[i])
	}
	block, err := des.NewCipher(key)
	if err != nil {
		return nil
	}
	out := make([]byte, 16)
	block.Encrypt(out[:8], challenge[:8])
	block.Encrypt(out[8:], challenge[8:])
	return out
}

func reverseBits(b byte) byte {
	b = (b&0xf0)>>4 | (b&0x0f)<<4
	b = (b&0xcc)>>2 | (b&0x33)<<2
	b = (b&0xaa)>>1 | (b&0x55)<<1
	return b
}
