//go:build !gui

// The default (headless) build has no GUI: the daemon is cgo-free and links no
// GTK. Build with `-tags gui` (needs libgtk-4-dev + libadwaita-1-dev) for the
// real libadwaita window in app.go.
package gui

import (
	"errors"

	"abacad-linux/internal/x11"
)

// Run reports that this binary was built without the GUI.
func Run(initialURL string, x *x11.Conn) error {
	_ = initialURL
	_ = x
	return errors.New("this abacad build has no GUI — rebuild with `-tags gui` (needs GTK4 + libadwaita)")
}
