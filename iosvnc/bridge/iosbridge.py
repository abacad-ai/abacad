#!/usr/bin/env python3
"""One process, one device session: frames out, touch in.

Why one process: iOS only delivers HID reports from a process that holds a
display stream, and the surface is only matched *after* that stream settles. A
viewer that opens the stream in one process and HID in another gets a state that
works sometimes — which is exactly how this project started.

Wire format, so the Go side stays trivial:

  stdout  length-prefixed frames: 4-byte big-endian length, then JPEG bytes
  stdin   JSON lines, one command each:
            {"op":"down"|"move"|"up"|"tap","x":0..65535,"y":0..65535}
            {"op":"key","text":"..."}   (not implemented yet)
  stderr  human-readable status and errors

Coordinates are normalised for the device's native display; the caller maps
framebuffer pixels to that range.
"""

import asyncio
import io
import os
import queue
import threading
import json
import struct
import sys
import time
import traceback

from PIL import Image

from pymobiledevice3.remote import native_tunnel
from pymobiledevice3.remote.core_device.display_service import DisplayService
from pymobiledevice3.remote.core_device.hid_service import (
    TOUCHSCREEN_STATE_CONTACT,
    TOUCHSCREEN_STATE_RELEASE,
    UniversalHIDServiceService,
)
from pymobiledevice3.remote.core_device.screen_stream import (
    depacketize_hevc,
    open_media_receiver,
)
from pymobiledevice3.remote.core_device.vt_jpeg import HevcToBgraTranscoder
from pymobiledevice3.services.power_assertion import PowerAssertionService

# How long to wait after the stream starts before touching HID: backboardd
# re-matches the surfaces asynchronously and a touch sent before that is
# dropped. Measured upstream: 0.1 s on an iPhone 15 Pro, 0.4 s on an iPhone 11.
AUTH_SETTLE = 0.5
TAP_DWELL = 0.08
JPEG_QUALITY = 70
MAX_FPS = 20
# Output width in pixels. 707 is 0.6 of an iPhone 15 Pro's 1179: UI text stays
# legible in a browser tab and a frame costs ~100 KB instead of ~220 KB.
OUTPUT_WIDTH = 707


def log(msg: str) -> None:
    print(msg, file=sys.stderr, flush=True)


def emit_frame(jpeg: bytes) -> None:
    sys.stdout.buffer.write(struct.pack(">I", len(jpeg)))
    sys.stdout.buffer.write(jpeg)
    sys.stdout.buffer.flush()


# Tunnels are kept reachable for the life of the process: PyTCP's stack is a
# process-global singleton, and dropping the handle would close the only route
# to the device.
_tunnels: list = []


async def _wifi_provider(udid: str | None):
    """A tunnel provider that reaches the device through usbmuxd's Network entry.

    This is the whole difference between USB and Wi-Fi: the same CoreDevice
    tunnel proxy, opened over a lockdown connection that usbmuxd carries to the
    device's Wi-Fi address instead of the USB one.
    """
    from pymobiledevice3.lockdown import create_using_usbmux
    from pymobiledevice3.remote.tunnel_service import CoreDeviceTunnelProxy
    from pymobiledevice3.remote.userspace_tunnel import TunnelProvider

    lockdown = await create_using_usbmux(serial=udid, connection_type="Network", autopair=True)
    try:
        return TunnelProvider(await CoreDeviceTunnelProxy.create(lockdown), lockdown)
    except BaseException:
        await lockdown.close()
        raise


async def establish_rsd(udid: str | None, connection: str = "usb"):
    """Return an RSD for the device over USB (default) or Wi-Fi.

    Both paths are no-root userspace tunnels; USB is the native (remoted
    piggyback) fallback's territory too, Wi-Fi is not. A Wi-Fi device has to be
    paired for the network already: on the Mac, "Connect to this iPhone over
    network" (lockdown -wifi-connections on), which makes usbmuxd list it twice,
    once per interface.
    """
    from pymobiledevice3.remote import userspace_tunnel

    if connection == "wifi":
        tunnel = userspace_tunnel.UserspaceRsdTunnel(
            serial=udid, provider_factory=lambda: _wifi_provider(udid)
        )
        try:
            rsd = await tunnel.aopen()
        except Exception as e:
            raise RuntimeError(
                f"no device of this UDID answered over Wi-Fi ({type(e).__name__}: {e}). "
                "Check that it is on the same network and that 'Connect to this iPhone "
                "over network' is enabled (pymobiledevice3 lockdown wifi-connections on)."
            ) from e
        _tunnels.append(tunnel)
        return rsd

    # Userspace first: it needs no root and it does not race remoted for the
    # NCM link. The native path is kept as a fallback for hosts where the
    # userspace stack cannot be built, and it is slow to fail (a 60 s timeout),
    # so it must not be the first thing tried.
    try:
        return await userspace_tunnel.establish_userspace_rsd(serial=udid)
    except Exception as e:
        log(f"userspace tunnel failed ({type(e).__name__}: {e}); trying native")
    return await native_tunnel.establish_native_rsd(serial=udid)


class FramePump:
    """RTP/HEVC -> BGRA -> scaled JPEG, encoded off the decoder thread."""

    def __init__(self, out_width: int = OUTPUT_WIDTH) -> None:
        self.vps = self.sps = self.pps = None
        self.transcoder: HevcToBgraTranscoder | None = None
        self.annexb: list[bytes] = []
        self.fu = bytearray()
        self.nals: list[bytes] = []
        self.last_emit = 0.0
        self.frames = 0
        self.size: tuple[int, int] | None = None
        self.out_width = out_width
        # VideoToolbox delivers frames in bursts, so a "skip while encoding"
        # policy drops most of them. Keep one pending frame instead and always
        # encode the freshest: latency stays bounded, nothing is starved.
        self._queue: queue.Queue = queue.Queue(maxsize=1)
        self._worker = threading.Thread(target=self._encode_loop, daemon=True)
        self._worker.start()
        self.packets = 0
        self.decoded = 0
        self.highest_seq = 0
        self.last_seq: int | None = None
        self.corrupt = False
        self.need_keyframe = False
        self.aus = 0
        self.dropped = 0

    def on_frame(self, bgra: bytes) -> None:
        self.decoded += 1
        if self.size is None:
            return
        w, h = self.size
        if len(bgra) < w * h * 4:
            return
        try:
            self._queue.put_nowait((bgra, w, h))
        except queue.Full:
            try:
                self._queue.get_nowait()
            except queue.Empty:
                pass
            try:
                self._queue.put_nowait((bgra, w, h))
            except queue.Full:
                pass

    def _encode_loop(self) -> None:
        while True:
            bgra, w, h = self._queue.get()
            self._encode(bgra, w, h)

    def _encode(self, bgra: bytes, w: int, h: int) -> None:
        started = time.monotonic()
        try:
            img = Image.frombuffer("RGBA", (w, h), bgra, "raw", "BGRA", 0, 1)
            # Fast box reduction first (C loop), then a small bilinear step to
            # land exactly on out_width. A one-shot resize of 3 M pixels costs
            # several hundred ms and caps the whole pipeline at a few fps.
            factor = max(1, round(w / float(self.out_width))) if self.out_width else 1
            if factor > 1:
                img = img.reduce(factor)
                w, h = img.size
            # No upscaling: if the integer reduction landed below the target,
            # the smaller frame is the better trade (fewer pixels to encode and
            # to push) -- the viewer scales to fit anyway.
            buf = io.BytesIO()
            img.convert("RGB").save(buf, "JPEG", quality=JPEG_QUALITY)
            emit_frame(buf.getvalue())
            self.frames += 1
            if self.frames <= 3 or self.frames % 25 == 0:
                log(f"frame {self.frames}: {len(buf.getbuffer())/1024:.0f} KB in {(time.monotonic()-started)*1000:.0f} ms")
        except Exception as e:
            log(f"encode failed: {type(e).__name__}: {e}")

    def feed_packet(self, packet: bytes) -> None:
        """Parse one RTP packet into the current access unit.

        The sequence-gap check is what keeps this usable: iOS streams over UDP
        and one lost packet corrupts the FU assembly for a whole access unit.
        Feeding that to VideoToolbox poisons the decoder -- it goes quiet rather
        than erroring, which looks exactly like "the device stopped sending".
        Drop the AU and ask the encoder for a fresh IDR instead.
        """
        if len(packet) < 12:
            return
        pt = packet[1] & 0x7F
        if 64 <= pt <= 95:  # interleaved RTCP
            return
        marker = bool(packet[1] & 0x80)
        cc = packet[0] & 0x0F
        off = 12 + cc * 4
        if packet[0] & 0x10:  # header extension
            if len(packet) < off + 4:
                return
            ext_words = int.from_bytes(packet[off + 2 : off + 4], "big")
            off += 4 + ext_words * 4
        if len(packet) <= off:
            return
        payload = packet[off:]

        seq = int.from_bytes(packet[2:4], "big")
        cur = self.highest_seq
        cycles = (cur >> 16) & 0xFFFF
        last16 = cur & 0xFFFF
        if seq < last16 and (last16 - seq) > 0x8000:
            cycles = (cycles + 1) & 0xFFFF
        ext = (cycles << 16) | seq
        if cur == 0 or ((ext - cur) & 0xFFFFFFFF) < 0x80000000:
            self.highest_seq = ext

        if self.last_seq is not None and seq != ((self.last_seq + 1) & 0xFFFF):
            self.fu.clear()
            self.corrupt = True
        if self.last_seq is None or ((seq - self.last_seq) & 0xFFFF) < 0x8000:
            self.last_seq = seq

        self.packets += 1
        self.nals.clear()
        depacketize_hevc(payload, self.fu, self.nals)
        for nal in self.nals:
            if not nal:
                continue
            nt = (nal[0] >> 1) & 0x3F
            if nt == 32:
                self.vps = bytes(nal)
            elif nt == 33:
                self.sps = bytes(nal)
            elif nt == 34:
                self.pps = bytes(nal)
            self.annexb.append(bytes(nal))

        if not marker:
            return
        self.aus += 1
        if self.corrupt:
            self.corrupt = False
            self.dropped += 1
            self.need_keyframe = True
            self.annexb.clear()
            return
        if not self.annexb:
            return
        if self.transcoder is None and self.vps and self.sps and self.pps:
            self.transcoder = HevcToBgraTranscoder(
                self.vps, self.sps, self.pps, on_frame=self.on_frame
            )
            log("decoder started")
        if self.transcoder is not None:
            self.transcoder.feed(b"\x00\x00\x00\x01" + b"\x00\x00\x00\x01".join(self.annexb))
        self.annexb.clear()


async def main() -> int:
    udid = sys.argv[1] if len(sys.argv) > 1 else None
    connection = os.environ.get("IOSVNC_CONNECTION", "usb").strip().lower() or "usb"

    rsd = await establish_rsd(udid, connection)
    log(f"rsd up: {rsd.udid} over {connection}")

    display = DisplayService(rsd)
    await display.connect()
    transport, receiver_ip = open_media_receiver(display, (8 * 1024 * 1024, 4 * 1024 * 1024))
    answer = await display.start_video_stream(
        receiver_ip=receiver_ip,
        receiver_port=transport.port,
        sender_ip=rsd.service.address[0],
        display_id=1,
        client_session_id=__import__("uuid").uuid4(),
        allow_rtcp_fb=True,
        ltrp_enabled=False,
    )
    log("video stream started")

    pump = FramePump()
    cfg = (answer or {}).get("connection", {}).get("streamConfig", {})
    sender_ip = rsd.service.address[0]
    source_port = int(cfg.get("SourcePort", 0) or 0)
    local_ssrc = int(cfg.get("RemoteSSRC", 0) or 0)
    remote_ssrc = int(cfg.get("LocalSSRC", 0) or 0)
    rtcp_dest = (sender_ip, source_port) if source_port else None
    pump.size = (int(cfg.get("CustomWidth", 1179)), int(cfg.get("CustomHeight", 2556)))
    log(f"encoder {pump.size[0]}x{pump.size[1]}")

    await asyncio.sleep(AUTH_SETTLE)
    uhs = UniversalHIDServiceService(rsd)
    await uhs.connect()
    service_id = await uhs.touchscreen_service_id(None)
    log(f"hid ready: touchscreen {service_id}")

    async def receive_loop() -> None:
        while True:
            packet = await transport.recv()
            pump.feed_packet(packet)
            # The userspace stack can return buffered packets without yielding,
            # which starves the input coroutine and turns taps into long presses.
            if pump.packets % 16 == 0:
                await asyncio.sleep(0)

    async def heartbeat() -> None:
        """Log once every two seconds, whether or not the screen is changing.

        The parent decides whether this process is alive from this line, so it
        must not depend on the device sending anything: a locked or still screen
        sends no video at all, and a stats line driven by packet arrival would
        make a healthy idle session look wedged.
        """
        last = time.monotonic()
        while True:
            await asyncio.sleep(2.0)
            now = time.monotonic()
            dt = max(1e-6, now - last)
            last = now
            log(
                f"stats: {pump.packets/dt:.0f} pkt/s, aus {pump.aus/dt:.0f}/s, "
                f"corrupt {pump.dropped/dt:.0f}/s, decoder {pump.decoded/dt:.1f} fps, "
                f"emitted {pump.frames/dt:.1f} fps"
            )
            pump.packets = 0
            pump.decoded = 0
            pump.frames = 0
            pump.aus = 0
            pump.dropped = 0

    def build_rr() -> bytes:
        rr = struct.pack("!BBHII BBBB IIII", 0x81, 0xC9, 7,
                         local_ssrc & 0xFFFFFFFF, remote_ssrc & 0xFFFFFFFF,
                         0, 0, 0, 0, pump.highest_seq & 0xFFFFFFFF, 0, 0, 0)
        sdes = struct.pack("!BBHI BBBB", 0x81, 0xCA, 2, local_ssrc & 0xFFFFFFFF, 0x01, 0, 0, 0)
        return rr + sdes

    def build_pli() -> bytes:
        return struct.pack("!BBHII", 0x81, 0xCE, 2,
                           local_ssrc & 0xFFFFFFFF, remote_ssrc & 0xFFFFFFFF)

    async def rtcp_loop() -> None:
        """Keep the stream alive: without a receiver report every second the
        device reaps the video session (~25 s timeout) and stops sending AUs."""
        prev = -1
        if rtcp_dest:
            await transport.sendto(build_pli(), *rtcp_dest)
            log(f"rtcp: dest={rtcp_dest} ours={local_ssrc} theirs={remote_ssrc}")
        while True:
            await asyncio.sleep(1.0)
            if not rtcp_dest:
                continue
            try:
                await transport.sendto(build_rr(), *rtcp_dest)
                if pump.decoded == prev or pump.need_keyframe:
                    pump.need_keyframe = False
                    await transport.sendto(build_pli(), *rtcp_dest)
                prev = pump.decoded
            except OSError as e:
                log(f"rtcp send failed: {e}")

    async def keep_awake() -> None:
        """Hold a PreventUserIdleSystemSleep assertion on the device.

        Without it iOS follows its auto-lock timer (often 2 minutes), the
        display sleeps, AUs stop, and a locked device will not start a fresh
        display session -- the bridge goes silently dead until someone picks the
        phone up. The assertion is renewed well inside its own timeout.
        """
        while True:
            try:
                svc = PowerAssertionService(rsd)
                async with svc.create_power_assertion(
                    "PreventUserIdleSystemSleep",
                    "iosvnc",
                    300,
                    "iosvnc keeping the device awake for the session",
                ):
                    await asyncio.sleep(120)
            except asyncio.CancelledError:
                return
            except Exception as e:
                log(f"keep-awake failed ({type(e).__name__}: {e}); retrying")
                await asyncio.sleep(15)

    contact_started = [0.0]

    async def command_loop() -> None:
        while True:
            line = await asyncio.to_thread(sys.stdin.readline)
            if not line:
                return
            try:
                cmd = json.loads(line)
            except json.JSONDecodeError:
                continue
            op = cmd.get("op")
            x = max(0, min(65535, int(cmd.get("x", 0))))
            y = max(0, min(65535, int(cmd.get("y", 0))))
            if op == "down":
                contact_started[0] = time.monotonic()
                await uhs.send_touchscreen(TOUCHSCREEN_STATE_CONTACT, x, y, service_id)
            elif op == "move":
                await uhs.send_touchscreen(TOUCHSCREEN_STATE_CONTACT, x, y, service_id)
            elif op == "up":
                # A contact shorter than a tap is ignored by iOS, and every
                # viewer sends down/up as fast as it can, so enforce the dwell
                # here instead of hoping each client does.
                held = time.monotonic() - contact_started[0]
                if 0 <= held < TAP_DWELL:
                    await asyncio.sleep(TAP_DWELL - held)
                await uhs.send_touchscreen(TOUCHSCREEN_STATE_RELEASE, x, y, service_id)
            elif op == "tap":
                await uhs.send_touchscreen(TOUCHSCREEN_STATE_CONTACT, x, y, service_id)
                await asyncio.sleep(TAP_DWELL)
                await uhs.send_touchscreen(TOUCHSCREEN_STATE_RELEASE, x, y, service_id)

    tasks = [
        asyncio.create_task(receive_loop()),
        asyncio.create_task(heartbeat()),
        asyncio.create_task(command_loop()),
        asyncio.create_task(rtcp_loop()),
        asyncio.create_task(keep_awake()),
    ]
    done, pending = await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
    for t in pending:
        t.cancel()
    for t in done:
        exc = t.exception()
        if exc is not None:
            log(f"fatal: {type(exc).__name__}: {exc}")
            raise exc
    return 0


if __name__ == "__main__":
    # Deliberately not asyncio.run(): its shutdown joins the default executor,
    # and the decoder's VideoToolbox threads can block that join for minutes.
    # Our stdout is the parent's liveness signal, so a bridge that lingers after
    # a dropped tunnel keeps the Go side serving a frozen screenshot -- the
    # "connected but nothing works" state. Exit hard instead: the pipe closes at
    # once and the supervisor starts a fresh session.
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    rc = 1
    try:
        rc = loop.run_until_complete(main())
    except KeyboardInterrupt:
        rc = 0
    except BaseException:
        traceback.print_exc()
    sys.stderr.flush()
    try:
        sys.stdout.buffer.flush()
    except Exception:
        pass
    os._exit(rc or 0)
