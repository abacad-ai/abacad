// Command iosvnc exposes a paired iPhone or iPad as a standard RFB (VNC)
// endpoint, without jailbreaking it.
//
// It is a standalone bridge: abacad, RealVNC, noVNC or any other viewer talks
// plain RFB to it, and it drives the device through Apple's developer services
// (CoreDevice display capture and HID injection).
//
// iOS 17 and later need a kernel tunnel owned by root:
//
//	sudo ios tunnel start
//
// where "ios" is the go-ios CLI (go install github.com/danielpaulus/go-ios@main).
// Run it once per boot; iosvnc connects through it.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/danielpaulus/go-ios/ios"

	"iosvnc/internal/frames"
	"iosvnc/internal/helper"
	"iosvnc/internal/iosinput"
	"iosvnc/internal/mediagate"
	"iosvnc/internal/rfbserver"
)

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "devices":
		runDevices()
	case "probe":
		runProbe(os.Args[2:])
	case "selftest":
		runSelftest(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `iosvnc - expose a paired iPhone/iPad as a VNC endpoint

  iosvnc doctor  [-device UDID] [-tunnel-info ADDR]
        check every prerequisite in order and name the missing one

  iosvnc devices
        list paired devices with their UDIDs

  iosvnc probe  [-device UDID] [-out shot.png]
        take one screenshot and report size and round-trip time

  iosvnc selftest [-device UDID] [-tap]
        run every stage once with timings: capture, frame rate, touch

  iosvnc serve  [-device UDID] [-listen :5909] [-password PW]
                [-scale 1.0] [-fps 0] [-quality 70] [-name NAME] [-synthetic]
        serve the device over RFB until interrupted

iOS 17+ needs a root kernel tunnel: sudo ios tunnel start
`)
	os.Exit(2)
}

func runDevices() {
	list, err := ios.ListDevices()
	if err != nil {
		fail(err)
	}
	if len(list.DeviceList) == 0 {
		fmt.Println("no paired devices")
		return
	}
	for _, d := range list.DeviceList {
		name, product, version := deviceLabels(d)
		fmt.Printf("%s  %-24s %-12s iOS %-8s %s\n",
			d.Properties.SerialNumber, name, product, version, d.ConnectionTypeLabel())
	}
}

func runProbe(args []string) {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	udid := fs.String("device", "", "device UDID (default: first paired device)")
	out := fs.String("out", "", "write the PNG here")
	_ = fs.Parse(args)

	device := openDevice(*udid)
	src, err := frames.OpenScreenshot(device, 0)
	if err != nil {
		fail(err)
	}
	defer src.Close()

	w, h := src.Size()
	fmt.Printf("device %s: %dx%d\n", device.Properties.SerialNumber, w, h)

	start := time.Now()
	frame, err := src.Wait(context.Background(), 0)
	if err != nil {
		fail(err)
	}
	fmt.Printf("first frame in %v\n", time.Since(start))
	if *out != "" {
		if err := writePNG(*out, frame.Image); err != nil {
			fail(err)
		}
		fmt.Printf("wrote %s\n", *out)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	udid := fs.String("device", "", "device UDID (default: first paired device)")
	listen := fs.String("listen", ":5909", "RFB listen address")
	password := fs.String("password", "", "require this VNC password (first 8 chars)")
	scale := fs.Float64("scale", 1.0, "output scale, 0 < scale <= 1")
	fps := fs.Int("fps", 15, "screenshot poll cap in frames per second (0 = as fast as the device answers)")
	quality := fs.Int("quality", 70, "Tight/JPEG quality, 1-100")
	name := fs.String("name", "", "desktop name shown to viewers")
	synthetic := fs.Bool("synthetic", false, "serve a test pattern instead of a device (no device or tunnel needed)")
	helperScript := fs.String("helper", "", "run the Python bridge at this path (bridge/iosbridge.py) to own the device session")
	helperPython := fs.String("helper-python", "", "interpreter for -helper (default: iosvnc/.venv/bin/python if present, else python3)")
	helperUDID := fs.String("udid", "", "device UDID for -helper")
	_ = fs.Parse(args)

	// Both real and synthetic sources are an rfbserver.Source plus a Run loop.
	var src rfbserver.Source
	var runner interface{ Run(context.Context) }
	var in rfbserver.Input

	if *synthetic {
		p := frames.NewPattern(1179, 2556)
		src, runner = p, p
		log.Printf("synthetic source %dx%d, no device needed", 1179, 2556)
	} else if *helperScript != "" {
		python := *helperPython
		if python == "" {
			python = "python3"
		}
		hc, err := helper.Start(context.Background(), python, *helperScript, *helperUDID, 75*time.Second)
		if err != nil {
			fail(err)
		}
		defer hc.Close()
		src, runner = hc, helperRunner{c: hc}
		nativeW, nativeH := hc.Size()
		outW, outH := rfbserver.OutputSize(nativeW, nativeH, *scale)
		in = hc.NewInput(outW, outH)
		log.Printf("bridge %s: %dx%d -> %dx%d", *helperScript, nativeW, nativeH, outW, outH)
	} else {
		device := openDevice(*udid)
		if *name == "" {
			if n, _, _ := deviceLabels(device); n != "" {
				*name = n
			}
		}
		shot, err := frames.OpenScreenshot(device, *fps)
		if err != nil {
			fail(err)
		}
		defer shot.Close()
		src, runner = shot, shot

		// Touch is only delivered while a media stream from this process is
		// running, so open one before the digitizer session.
		gate, gateErr := mediagate.Open(device, 0)
		if gateErr != nil {
			log.Printf("warning: no media stream (%v); the device will drop every touch event", gateErr)
		} else {
			defer gate.Close()
			log.Printf("media stream open: touch is authenticated")
			time.Sleep(mediagate.AuthSettle)
			go func() {
				<-gate.Done()
				if err := gate.Err(); err != nil {
					log.Printf("warning: media stream ended (%v); touch no longer reaches the device", err)
				}
			}()
		}

		nativeW, nativeH := shot.Size()
		outW, outH := rfbserver.OutputSize(nativeW, nativeH, *scale)
		touch, err := iosinput.NewTouch(device, outW, outH)
		if err != nil {
			// Viewing is still useful when input is unavailable (an older
			// runtime, or a device whose digitizer service is missing), so
			// degrade instead of refusing to serve.
			log.Printf("warning: touch unavailable, serving view-only: %v", err)
		} else {
			defer touch.Close()
			in = touch
		}
	}

	if *name == "" {
		*name = "iosvnc"
	}
	srv := rfbserver.New(src, in, rfbserver.Options{
		Name:     *name,
		Password: *password,
		Scale:    *scale,
		Quality:  *quality,
		Logf:     log.Printf,
	})
	outW, outH := srv.OutputSize()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go runner.Run(ctx)

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fail(err)
	}
	defer ln.Close()
	log.Printf("%s listening on %s (RFB, %dx%d%s)", *name, ln.Addr(), outW, outH, authNote(*password))

	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("viewer %s connected", conn.RemoteAddr())
			if err := srv.ServeConn(conn); err != nil {
				log.Printf("viewer %s: %v", conn.RemoteAddr(), err)
			}
			log.Printf("viewer %s disconnected", conn.RemoteAddr())
		}()
	}
	wg.Wait()
}

// runSelftest exercises every stage of the bridge in order, so bringing up a new
// device is one command with a clear failure point instead of a debugging
// session.
func runSelftest(args []string) {
	fs := flag.NewFlagSet("selftest", flag.ExitOnError)
	udid := fs.String("device", "", "device UDID (default: first paired device)")
	doTap := fs.Bool("tap", true, "touch the centre of the screen")
	_ = fs.Parse(args)

	device := openDevice(*udid)
	name, product, version := deviceLabels(device)
	fmt.Printf("1. device       %s (%s, iOS %s) %s\n",
		name, product, version, device.Properties.SerialNumber)

	start := time.Now()
	src, err := frames.OpenScreenshot(device, 10)
	if err != nil {
		fail(err)
	}
	defer src.Close()
	w, h := src.Size()
	fmt.Printf("2. capture      %dx%d, first frame %v\n", w, h, time.Since(start).Round(time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go src.Run(ctx)

	first, err := src.Wait(ctx, 0)
	if err != nil {
		fail(err)
	}
	stepStart := time.Now()
	second, err := src.Wait(ctx, first.Seq)
	if err != nil {
		fail(err)
	}
	fmt.Printf("3. frame rate   %v per frame (~%.1f fps)\n",
		time.Since(stepStart).Round(time.Millisecond), 1/time.Since(stepStart).Seconds())

	if !*doTap {
		fmt.Println("4. touch        skipped (-tap=false)")
		fmt.Println("selftest: capture path ok")
		return
	}

	gate, err := mediagate.Open(device, 0)
	if err != nil {
		log.Printf("warning: no media stream (%v); touch will be dropped by the device", err)
	} else {
		defer gate.Close()
		time.Sleep(mediagate.AuthSettle)
		fmt.Printf("4. stream       media stream open, %d RTP packets so far\n", gate.Packets())
	}

	touch, err := iosinput.NewTouch(device, w, h)
	if err != nil {
		fail(err)
	}
	defer touch.Close()

	tapStart := time.Now()
	cx, cy := w/2, h/2
	if err := touch.Pointer(1, cx, cy); err != nil {
		fail(err)
	}
	if err := touch.Pointer(0, cx, cy); err != nil {
		fail(err)
	}
	fmt.Printf("4. touch        tapped (%d,%d) in %v\n", cx, cy, time.Since(tapStart).Round(time.Millisecond))

	third, err := src.Wait(ctx, second.Seq)
	if err != nil {
		fail(err)
	}
	if frameHash(second.Image) == frameHash(third.Image) {
		fmt.Println("5. reaction     screen unchanged after the tap (the centre may not be tappable)")
	} else {
		fmt.Println("5. reaction     screen changed after the tap")
	}
	fmt.Println("selftest: ok - viewer can connect with 'iosvnc serve'")
}

func frameHash(img image.Image) string {
	b := img.Bounds()
	h := sha256.New()
	row := make([]byte, 0, b.Dx()*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row = row[:0]
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			row = append(row, byte(r>>8), byte(g>>8), byte(bl>>8), 0)
		}
		h.Write(row)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// helperRunner keeps the RFB server alive for as long as the bridge process is;
// the frames themselves arrive on the client's own read loop.
type helperRunner struct{ c *helper.Client }

func (r helperRunner) Run(ctx context.Context) {
	<-ctx.Done()
	r.c.Close()
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// deviceLabels reads the human-facing name, product type and OS version from
// lockdown. They are cosmetic, so a failure degrades to blanks.
func deviceLabels(d ios.DeviceEntry) (name, product, version string) {
	vals, err := ios.GetValues(d)
	if err != nil {
		return "", "", ""
	}
	return vals.Value.DeviceName, vals.Value.ProductType, vals.Value.ProductVersion
}

func authNote(password string) string {
	if password == "" {
		return ", no password"
	}
	return ", password required"
}

// openDevice resolves a UDID, a serial number, or a case-insensitive fragment of
// the device name. Typing "iphone" is friendlier than a UDID.
func openDevice(selector string) ios.DeviceEntry {
	device, err := resolveDevice(selector)
	if err != nil {
		fail(err)
	}
	return device
}

// resolveDevice is openDevice without the exit, so doctor can report the failure
// as one missing prerequisite instead of aborting.
func resolveDevice(selector string) (ios.DeviceEntry, error) {
	list, err := ios.ListDevices()
	if err != nil {
		return ios.DeviceEntry{}, err
	}
	if len(list.DeviceList) == 0 {
		return ios.DeviceEntry{}, fmt.Errorf("no iOS device is visible to usbmuxd")
	}
	if strings.TrimSpace(selector) == "" {
		return list.DeviceList[0], nil
	}
	if device, err := ios.GetDevice(selector); err == nil {
		return device, nil
	}
	want := strings.ToLower(selector)
	for _, d := range list.DeviceList {
		name, product, _ := deviceLabels(d)
		if strings.Contains(strings.ToLower(name), want) || strings.Contains(strings.ToLower(product), want) {
			return d, nil
		}
	}
	return ios.DeviceEntry{}, fmt.Errorf("no paired device matches %q", selector)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "iosvnc: %v\n", err)
	fmt.Fprintf(os.Stderr, "hint: iOS 17+ needs a root tunnel - run 'sudo ios tunnel start' in another terminal, unlock the device, and try again\n")
	os.Exit(1)
}
