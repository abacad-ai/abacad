package rfb

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestNegotiateARD(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	_ = server.SetDeadline(time.Now().Add(5 * time.Second))
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))

	serverErr := make(chan error, 1)
	go func() { serverErr <- mockARDServer(server) }()

	in := bufio.NewReader(client)
	banner, err := readBanner(in)
	if err != nil {
		t.Fatal(err)
	}
	if banner != "RFB 003.889" {
		t.Fatalf("banner = %q", banner)
	}
	if _, err := io.WriteString(client, "RFB 003.008\n"); err != nil {
		t.Fatal(err)
	}
	if err := negotiateUpstream(in, client, "RFB 003.008\n", "pässwörd", "li", true); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func mockARDServer(conn net.Conn) error {
	if _, err := io.WriteString(conn, "RFB 003.889\n"); err != nil {
		return err
	}
	in := bufio.NewReader(conn)
	ver, err := in.ReadString('\n')
	if err != nil {
		return err
	}
	if ver != "RFB 003.008\n" {
		return fmt.Errorf("client version = %q", ver)
	}
	// Offer the legacy password and Apple DH; the proxy must select DH.
	if _, err := conn.Write([]byte{2, 2, securityTypeARD}); err != nil {
		return err
	}
	chosen, err := in.ReadByte()
	if err != nil {
		return err
	}
	if chosen != securityTypeARD {
		return fmt.Errorf("security type = %d", chosen)
	}

	prime, err := rand.Prime(rand.Reader, 512)
	if err != nil {
		return err
	}
	generator := big.NewInt(2)
	private, err := rand.Int(rand.Reader, new(big.Int).Sub(prime, big.NewInt(3)))
	if err != nil {
		return err
	}
	private.Add(private, big.NewInt(2))
	public := new(big.Int).Exp(generator, private, prime)

	const keyLen = 64
	var header [4]byte
	binary.BigEndian.PutUint16(header[0:2], uint16(generator.Int64()))
	binary.BigEndian.PutUint16(header[2:4], keyLen)
	if _, err := conn.Write(header[:]); err != nil {
		return err
	}
	if _, err := conn.Write(leftPad(prime.Bytes(), keyLen)); err != nil {
		return err
	}
	if _, err := conn.Write(leftPad(public.Bytes(), keyLen)); err != nil {
		return err
	}

	creds := make([]byte, 128)
	if _, err := io.ReadFull(in, creds); err != nil {
		return err
	}
	clientPublic := make([]byte, keyLen)
	if _, err := io.ReadFull(in, clientPublic); err != nil {
		return err
	}

	shared := new(big.Int).Exp(new(big.Int).SetBytes(clientPublic), private, prime)
	digest := md5.Sum(leftPad(shared.Bytes(), keyLen))
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return err
	}
	for i := 0; i < len(creds); i += aes.BlockSize {
		block.Decrypt(creds[i:i+aes.BlockSize], creds[i:i+aes.BlockSize])
	}
	user := string(creds[:bytes.IndexByte(creds[:64], 0)])
	pass := string(creds[64 : 64+bytes.IndexByte(creds[64:], 0)])
	if user != "li" {
		return fmt.Errorf("username = %q", user)
	}
	if pass != "pässwörd" {
		return fmt.Errorf("password = %q", pass)
	}
	return binary.Write(conn, binary.BigEndian, uint32(0))
}

func TestNegotiateARDRequiresType30(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	_ = server.SetDeadline(time.Now().Add(2 * time.Second))
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))

	go func() {
		_, _ = io.WriteString(server, "RFB 003.889\n")
		in := bufio.NewReader(server)
		_, _ = in.ReadString('\n')
		_, _ = server.Write([]byte{2, 1, 2})
	}()
	in := bufio.NewReader(client)
	if _, err := readBanner(in); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(client, "RFB 003.008\n"); err != nil {
		t.Fatal(err)
	}
	err := negotiateUpstream(in, client, "RFB 003.008\n", "pw", "li", true)
	if err == nil || !strings.Contains(err.Error(), "type 30") {
		t.Fatalf("err = %v", err)
	}
}

func TestTruncateUTF8(t *testing.T) {
	s := strings.Repeat("é", 40) // 80 bytes
	got := truncateUTF8(s, 63)
	if len(got) > 63 {
		t.Fatalf("len = %d", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("split a rune: %q", got)
	}
	if len(got) != 62 {
		t.Fatalf("len = %d, want 62", len(got))
	}
}
