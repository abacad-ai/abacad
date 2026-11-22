package rfb

import (
	"bufio"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"unicode/utf8"
)

// securityTypeARD is Apple's Diffie-Hellman authentication, security type 30.
// macOS Screen Sharing offers it alongside its RSA tunnel variant (33) and the
// legacy VNC password (2). Type 30 is the one that carries a macOS account
// name: it is what puts the connection into that user's console session.
const securityTypeARD byte = 30

// ardAuth performs Apple's type 30 exchange:
//
//	S->C  generator (u16), key length (u16), prime (keyLen), server public key (keyLen)
//	C->S  AES-128-ECB(MD5(shared secret), username[64] + password[64]), client public key
//	S->C  SecurityResult
//
// The credentials are not a DES-truncated VNC password; they are the real
// macOS account name and password, up to 63 bytes each.
func ardAuth(in *bufio.Reader, out io.Writer, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("macOS account name is required for ARD authentication")
	}
	if password == "" {
		return errors.New("macOS account password is required for ARD authentication")
	}

	var params [4]byte
	if _, err := io.ReadFull(in, params[:]); err != nil {
		return fmt.Errorf("read ARD DH parameters: %w", err)
	}
	generator := new(big.Int).SetBytes(params[0:2])
	keyLen := int(binary.BigEndian.Uint16(params[2:4]))
	if keyLen < 64 || keyLen > 1024 {
		return fmt.Errorf("unsupported ARD DH key length %d", keyLen)
	}

	primeBytes := make([]byte, keyLen)
	if _, err := io.ReadFull(in, primeBytes); err != nil {
		return fmt.Errorf("read ARD DH prime: %w", err)
	}
	serverPubBytes := make([]byte, keyLen)
	if _, err := io.ReadFull(in, serverPubBytes); err != nil {
		return fmt.Errorf("read ARD DH public key: %w", err)
	}

	prime := new(big.Int).SetBytes(primeBytes)
	serverPub := new(big.Int).SetBytes(serverPubBytes)
	if prime.BitLen() < 512 || prime.Bit(0) == 0 {
		return errors.New("ARD DH prime is not a usable odd modulus")
	}
	two := big.NewInt(2)
	if generator.Cmp(two) < 0 || generator.Cmp(prime) >= 0 {
		return fmt.Errorf("ARD DH generator %s is out of range", generator)
	}
	upper := new(big.Int).Sub(prime, big.NewInt(2))
	if serverPub.Cmp(two) < 0 || serverPub.Cmp(upper) > 0 {
		return errors.New("ARD DH server public key is out of range")
	}

	// A full-size private exponent keeps the exchange unremarkable. 4096-bit
	// modular exponentiation is a few milliseconds in math/big.
	private, err := rand.Int(rand.Reader, new(big.Int).Sub(prime, big.NewInt(3)))
	if err != nil {
		return fmt.Errorf("generate ARD DH private key: %w", err)
	}
	private.Add(private, two)
	clientPub := new(big.Int).Exp(generator, private, prime)
	shared := new(big.Int).Exp(serverPub, private, prime)

	digest := md5.Sum(leftPad(shared.Bytes(), keyLen))
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return fmt.Errorf("initialize ARD AES key: %w", err)
	}

	creds := make([]byte, 128)
	if _, err := rand.Read(creds); err != nil {
		return err
	}
	user := []byte(truncateUTF8(username, 63))
	copy(creds[0:64], user)
	creds[len(user)] = 0
	pass := []byte(truncateUTF8(password, 63))
	copy(creds[64:128], pass)
	creds[64+len(pass)] = 0
	for i := 0; i < len(creds); i += aes.BlockSize {
		block.Encrypt(creds[i:i+aes.BlockSize], creds[i:i+aes.BlockSize])
	}

	if _, err := out.Write(creds); err != nil {
		return fmt.Errorf("write ARD credentials: %w", err)
	}
	if _, err := out.Write(leftPad(clientPub.Bytes(), keyLen)); err != nil {
		return fmt.Errorf("write ARD public key: %w", err)
	}
	return nil
}

// leftPad returns b as an n-byte big-endian integer. Diffie-Hellman values are
// fixed-width on the wire; a leading zero byte is significant.
func leftPad(b []byte, n int) []byte {
	if len(b) >= n {
		return b
	}
	out := make([]byte, n)
	copy(out[n-len(b):], b)
	return out
}

// truncateUTF8 cuts s to at most n bytes without splitting a rune. The ARD
// credential blob gives each field 64 bytes, one of which is the NUL.
func truncateUTF8(s string, n int) string {
	for len(s) > n {
		_, size := utf8.DecodeLastRuneInString(s)
		if size == 0 {
			break
		}
		s = s[:len(s)-size]
	}
	return s
}
