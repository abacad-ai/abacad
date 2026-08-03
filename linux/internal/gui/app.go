//go:build gui

// Package gui is the abacad Linux client's GTK4 / libadwaita front-end. It is
// built only with `-tags gui` (cgo + libgtk-4 + libadwaita); the default headless
// daemon build uses stub.go instead and stays cgo-free.
//
// The window follows the shared abacad client model: a live State header
// (Controlling now / Connected / Disconnected), "screen being watched" /
// "recording" flags, a Pause / Disconnect pair, the recent-actions tail, and a
// server-URL / Connect row. Native Adwaita chrome; our tokens (theme_gen.go)
// supply the status colors via Pango markup so libadwaita's own dark/light theme
// is left intact.
package gui

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"abacad-linux/internal/agent"
	"abacad-linux/internal/enroll"
	"abacad-linux/internal/status"
	"abacad-linux/internal/x11"
)

const appID = "ai.abacad.client"

// Run launches the GUI, driving connections through a Supervisor over x (the
// desktop X11 connection; may be nil on a headless box). Blocks until the window
// is closed.
func Run(initialURL string, x *x11.Conn, setup Setup) error {
	sup := agent.NewSupervisor(x)
	app := adw.NewApplication(appID, gio.ApplicationFlagsNone)
	app.ConnectActivate(func() { activate(app, sup, initialURL, setup) })
	if code := app.Run(nil); code != 0 {
		return fmt.Errorf("gtk application exited with code %d", code)
	}
	return nil
}

// palette resolves our tokens for the current Adwaita appearance.
func palette() Palette { return Of(adw.StyleManagerGetDefault().Dark()) }

func activate(app *adw.Application, sup *agent.Supervisor, initialURL string, setup Setup) {
	win := adw.NewApplicationWindow(&app.Application)
	win.SetTitle("abacad")
	win.SetDefaultSize(440, 600)

	// Content column inside an Adwaita toolbar view (flat header bar on top).
	toolbar := adw.NewToolbarView()
	toolbar.AddTopBar(adw.NewHeaderBar())

	body := gtk.NewBox(gtk.OrientationVertical, SpaceMd)
	body.SetMarginTop(SpaceLg)
	body.SetMarginBottom(SpaceLg)
	body.SetMarginStart(SpaceLg)
	body.SetMarginEnd(SpaceLg)

	// --- state header ---
	headline := gtk.NewLabel("")
	headline.SetXAlign(0)
	headline.SetUseMarkup(true)
	subtitle := gtk.NewLabel("")
	subtitle.SetXAlign(0)
	subtitle.AddCSSClass("dim-label")
	subtitle.SetWrap(true)

	watched := gtk.NewLabel("")
	watched.SetXAlign(0)
	watched.SetUseMarkup(true)
	recording := gtk.NewLabel("")
	recording.SetXAlign(0)
	recording.SetUseMarkup(true)

	// --- setup: claim code, or the button that asks for one ---
	//
	// Nothing here contacts the relay until setupBtn is clicked. That is the
	// whole point of the section: on a fresh install this window opens having
	// sent nothing anywhere, and says which relay it is about to talk to before
	// it talks to it — a self-hoster has to be able to change the address first.
	setupHint := gtk.NewLabel("")
	setupHint.SetXAlign(0)
	setupHint.SetWrap(true)
	setupHint.AddCSSClass("dim-label")

	claimLabel := gtk.NewLabel("")
	claimLabel.SetXAlign(0)
	claimLabel.SetUseMarkup(true)
	claimLabel.SetSelectable(true)
	claimLabel.SetWrap(true)

	// Left full-width like urlEntry rather than aligned: this file only compiles
	// on a machine with the GTK headers, so every gotk4 call in it is one already
	// exercised by a shipped build.
	setupBtn := gtk.NewButtonWithLabel("Set up this device")
	setupBtn.AddCSSClass("suggested-action")

	// Enrollment display state. Only ever touched on the GTK main thread — the
	// enrollment goroutine reaches it through glib.IdleAdd — so no mutex.
	var (
		enrolling  bool
		claimID    string
		claimCode  string
		enrollErr  string
		enrollDone bool // claimed and dialled; the setup section retires
	)

	// --- controls: URL + connect/disconnect/pause ---
	urlEntry := gtk.NewEntry()
	urlEntry.SetPlaceholderText("wss://host/device?token=…")
	if initialURL != "" {
		urlEntry.SetText(initialURL)
	}

	connectBtn := gtk.NewButtonWithLabel("Connect")
	connectBtn.AddCSSClass("suggested-action")
	disconnectBtn := gtk.NewButtonWithLabel("Disconnect")
	disconnectBtn.AddCSSClass("destructive-action")
	pauseBtn := gtk.NewButtonWithLabel("Pause")

	connectBtn.ConnectClicked(func() { sup.Connect(strings.TrimSpace(urlEntry.Text())) })
	disconnectBtn.ConnectClicked(func() { sup.Disconnect() })
	pauseBtn.ConnectClicked(func() { status.SetPaused(!status.Get().Paused) })

	btnRow := gtk.NewBox(gtk.OrientationHorizontal, SpaceSm)
	btnRow.Append(connectBtn)
	btnRow.Append(disconnectBtn)
	btnRow.Append(pauseBtn)

	// --- recent actions ---
	actionsLabel := gtk.NewLabel("Setup — enter the connection URL and Connect.")
	actionsLabel.SetXAlign(0)
	actionsLabel.SetWrap(true)
	actionsLabel.SetSelectable(true)

	sep := gtk.NewSeparator(gtk.OrientationHorizontal)

	body.Append(headline)
	body.Append(subtitle)
	body.Append(watched)
	body.Append(recording)
	body.Append(gtk.NewLabel("")) // spacer
	body.Append(setupHint)
	body.Append(claimLabel)
	body.Append(setupBtn)
	body.Append(urlEntry)
	body.Append(btnRow)
	body.Append(sep)
	body.Append(sectionLabel("Recent actions"))
	body.Append(actionsLabel)

	toolbar.SetContent(body)
	win.SetContent(toolbar)

	// render paints the current status onto the widgets; it runs on the GTK main
	// thread (status change → IdleAdd → render).
	render := func() {
		p := palette()
		s := status.Get()

		dot, title, sub := stateLine(s, p)
		headline.SetMarkup(fmt.Sprintf(`<span foreground="%s" weight="bold" size="large">%s %s</span>`,
			dot, "●", html.EscapeString(title)))
		subtitle.SetText(sub)

		watched.SetVisible(s.Watched)
		if s.Watched {
			watched.SetMarkup(fmt.Sprintf(`<span foreground="%s" weight="bold">👁 Screen being watched</span>`, p.Warning))
		}
		recording.SetVisible(s.Recording)
		if s.Recording {
			recording.SetMarkup(fmt.Sprintf(`<span foreground="%s" weight="bold">● Recording</span>`, p.Danger))
		}

		connected := s.State != status.Disconnected
		disconnectBtn.SetSensitive(connected)
		pauseBtn.SetSensitive(connected)
		if s.Paused {
			pauseBtn.SetLabel("Resume")
		} else {
			pauseBtn.SetLabel("Pause")
		}

		// Setup section. Hidden once this device is connected or has finished
		// enrolling; the URL entry stays as the manual/advanced route.
		showSetup := !connected && initialURL == "" && !enrollDone
		setupHint.SetVisible(showSetup)
		claimLabel.SetVisible(showSetup && claimCode != "")
		setupBtn.SetVisible(showSetup && !enrolling && setup.canAct())

		switch {
		case !showSetup:
		case enrollErr != "":
			setupHint.SetText(enrollErr)
			setupBtn.SetLabel("Try again")
		case claimCode != "":
			claimLabel.SetMarkup(fmt.Sprintf(
				"Device ID  <tt>%s</tt>\nClaim code  <tt><b>%s</b></tt>",
				html.EscapeString(claimID), html.EscapeString(claimCode)))
			setupHint.SetText(fmt.Sprintf(
				"Open %s/claim and enter both. The code changes every few minutes; anyone who can read both lines can claim this device.",
				setup.relay()))
		case enrolling:
			setupHint.SetText(fmt.Sprintf("Contacting %s…", setup.relay()))
		case !setup.canEnroll():
			// Nothing on screen to act on, and registering needs a display to
			// show a claim code. Say so rather than offering a button that
			// cannot succeed. Ordered last so it never contradicts a live code
			// or an in-flight resume, both of which work without one.
			setupHint.SetText("This machine has no display, so it can't show a claim code. Run `abacad connect` to add it, or paste a connection URL below.")
		default:
			setupHint.SetText(fmt.Sprintf(
				"This device hasn't been set up yet. Registers with %s and shows a code to enter there — nothing is sent until you press the button.",
				setup.relay()))
			setupBtn.SetLabel("Set up this device")
		}

		actionsLabel.SetText(formatLines(s.Lines))
	}

	// runSetup drives enrollment on a background goroutine, marshalling every UI
	// touch back to the GTK thread. allowRegister is false on the automatic
	// resume at startup and true only from the button — the whole gate.
	ctx, cancelSetup := context.WithCancel(context.Background())
	runSetup := func(allowRegister bool) {
		if enrolling || !setup.wired() {
			return
		}
		enrolling = true
		enrollErr = ""
		render()

		sess := *setup.Session // copy: we override the callbacks per run
		sess.OnCode = func(deviceID, code string) {
			glib.IdleAdd(func() {
				claimID, claimCode = deviceID, code
				render()
			})
		}
		sess.OnStatus = func(msg string) {
			glib.IdleAdd(func() { status.Event("• " + msg) })
		}

		go func() {
			res, err := sess.Run(ctx, setup.Token, setup.DeviceID, allowRegister)
			glib.IdleAdd(func() {
				enrolling = false
				switch {
				case errors.Is(err, enroll.ErrSetupRequired):
					// Expected on a fresh install: nothing was sent, and the
					// button is now the only way forward.
				case errors.Is(err, context.Canceled):
					// Window closing.
				case errors.Is(err, enroll.ErrNotSupported):
					enrollErr = "This relay doesn't support automatic setup. Run `abacad connect`, or paste a connection URL below."
				case err != nil:
					enrollErr = err.Error()
					// The displayed code has rotated by now; leaving it up
					// invites someone to type one the relay will reject.
					claimCode = ""
				default:
					// Claimed. Remember the credential so a retry within this
					// session doesn't re-register, then dial.
					setup.Token, setup.DeviceID = res.Token, res.DeviceID
					claimCode, enrollDone = "", true
					sup.Connect(setup.DialURL(res.DeviceURL, res.Token))
				}
				render()
			})
		}()
	}

	setupBtn.ConnectClicked(func() { runSetup(true) })

	unsub := status.Subscribe(func() {
		// Status writers run on the agent's goroutines; hop to the GTK thread.
		glib.IdleAdd(render)
	})
	win.ConnectCloseRequest(func() bool {
		cancelSetup()
		unsub()
		return false
	})

	render()
	win.Present()

	// Resume, don't register: with a credential already on disk this reconnects
	// with no clicks, which is the behaviour a set-up device had before. With
	// none it returns ErrSetupRequired without a single network call, and the
	// window just shows the button.
	if initialURL == "" && setup.canResume() {
		runSetup(false)
	}
}

// stateLine maps a snapshot to (dot color, title, subtitle).
func stateLine(s status.Snapshot, p Palette) (dot, title, sub string) {
	switch {
	case s.Paused:
		return p.Warning, "Paused", "commands are being rejected on this machine"
	case s.Controlling:
		return p.Success, "Controlling now", "agent · " + fallback(s.LastMethod, "running")
	case s.State == status.Connected:
		return p.Success, "Connected", "idle — no agent active"
	case s.State == status.Connecting:
		return p.Warning, "Connecting", s.Detail
	case s.State == status.Reconnecting:
		return p.Warning, "Reconnecting", s.Detail
	default:
		return p.InkSubtle, "Disconnected", s.Detail
	}
}

func sectionLabel(text string) *gtk.Label {
	l := gtk.NewLabel(text)
	l.SetXAlign(0)
	l.AddCSSClass("dim-label")
	l.AddCSSClass("caption-heading")
	return l
}

func formatLines(lines []status.Line) string {
	if len(lines) == 0 {
		return "No activity yet."
	}
	var b strings.Builder
	// Newest first, capped.
	n := len(lines)
	start := 0
	if n > 14 {
		start = n - 14
	}
	for i := n - 1; i >= start; i-- {
		ts := time.UnixMilli(lines[i].TS).Format("15:04:05")
		b.WriteString(ts)
		b.WriteString("  ")
		b.WriteString(lines[i].Text)
		if i > start {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
