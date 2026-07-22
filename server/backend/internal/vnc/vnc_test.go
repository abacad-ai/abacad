package vnc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"abacad/internal/store"
)

// TestBridgePipesRFB verifies the server relays RFB bytes both ways between the
// device's ingress socket and the browser's watch socket, and that the watch
// endpoint rejects an unknown ticket.
func TestBridgePipesRFB(t *testing.T) {
	m := NewManager(nil, "wss://test", func(r *http.Request) (store.Account, error) {
		return store.Account{ID: "acc1"}, nil
	})
	// Register a session directly (Start needs a device hub, out of scope here).
	s := &session{
		mgr: m, deviceID: "dev1", accountID: "acc1",
		ingressTok: "itok", ticket: "tkt",
		expiresAt: time.Now().Add(time.Minute),
		done:      make(chan struct{}),
	}
	m.byDevice["dev1"] = s
	m.byIngress["itok"] = s
	m.byTicket["tkt"] = s

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vnc/ingress":
			m.ServeIngress(w, r)
		case "/vnc/watch":
			m.ServeWatch(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	ws := "ws" + strings.TrimPrefix(srv.URL, "http")

	dev, _, err := websocket.Dial(ctx, ws+"/vnc/ingress?token=itok", nil)
	if err != nil {
		t.Fatalf("device dial: %v", err)
	}
	defer dev.Close(websocket.StatusNormalClosure, "")
	viewer, _, err := websocket.Dial(ctx, ws+"/vnc/watch?ticket=tkt", nil)
	if err != nil {
		t.Fatalf("viewer dial: %v", err)
	}
	defer viewer.Close(websocket.StatusNormalClosure, "")

	// device -> viewer (framebuffer direction)
	if err := dev.Write(ctx, websocket.MessageBinary, []byte("RFB 003.008\n")); err != nil {
		t.Fatal(err)
	}
	_, got, err := viewer.Read(ctx)
	if err != nil || string(got) != "RFB 003.008\n" {
		t.Fatalf("viewer got %q err %v", got, err)
	}

	// viewer -> device (input direction)
	if err := viewer.Write(ctx, websocket.MessageBinary, []byte("RFB 003.003\n")); err != nil {
		t.Fatal(err)
	}
	_, got, err = dev.Read(ctx)
	if err != nil || string(got) != "RFB 003.003\n" {
		t.Fatalf("device got %q err %v", got, err)
	}

	// A bad ticket is rejected.
	if _, _, err := websocket.Dial(ctx, ws+"/vnc/watch?ticket=nope", nil); err == nil {
		t.Fatal("expected watch with unknown ticket to fail")
	}

	// Single-use: the ticket was consumed when the viewer connected above, so
	// reusing it is rejected (even though the account cookie still matches).
	if _, _, err := websocket.Dial(ctx, ws+"/vnc/watch?ticket=tkt", nil); err == nil {
		t.Fatal("expected reuse of a consumed ticket to fail")
	}
}
