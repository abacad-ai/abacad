package rfb

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestSecurityTypes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "RFB 003.008\n")
		br := bufio.NewReader(conn)
		if _, err := br.ReadString('\n'); err != nil {
			return
		}
		conn.Write([]byte{3, 30, 33, 36})
	}()

	banner, types, err := SecurityTypes(context.Background(), ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if banner != "RFB 003.008" {
		t.Fatalf("banner = %q", banner)
	}
	if len(types) != 3 || types[0] != 30 {
		t.Fatalf("types = %v", types)
	}
	if SupportsStandardAuth(types) {
		t.Fatal("Apple-only types should not be considered standard")
	}
	if !SupportsStandardAuth([]byte{2}) || !SupportsStandardAuth([]byte{1, 2}) {
		t.Fatal("standard auth types should be supported")
	}
}
