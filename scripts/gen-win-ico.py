#!/usr/bin/env python3
"""Build windows/Abacad.ico from assets/icon-rounded-1024.png.

Run once and commit the .ico — the same deal as macos/AppIcon.icns, which is
committed rather than rebuilt in CI because `make macos-icon` needs a Mac. This
script needs nothing but the stdlib, but the principle holds: the Windows job on
the NUC should compile an installer, not rasterize artwork.

    python3 scripts/gen-win-ico.py

Rounded mark, not the circle: a Windows shortcut/taskbar tile is square, and the
circle mark is drawn for macOS's superellipse mask.

Stdlib-only (zlib is the whole dependency) because neither the dev box nor the
runners have Pillow or ImageMagick, and adding an image library to build one
committed file that changes ~never is a bad trade.
"""

import os
import struct
import sys
import zlib

# Windows picks the nearest size and downscales; supplying the exact sizes the
# shell asks for avoids its (poor) on-the-fly resampling. 16/32/48 are Explorer,
# 24 shows up in some list views, 64/128 in the larger icon modes, 256 is the
# extra-large tile and the one Inno's wizard header uses.
SIZES = [16, 24, 32, 48, 64, 128, 256]

# Entries at or below this edge length are stored as a classic BMP/DIB. Only the
# 256 is PNG-compressed. Vista+ reads PNG at any size, but the resource compiler
# behind <ApplicationIcon> and some older shell paths are still happiest with
# DIBs for the small frames, and the size cost is trivial.
PNG_ABOVE = 128


def decode_png(blob):
    """Return (width, height, rgba_bytes) for an 8-bit RGBA non-interlaced PNG."""
    if blob[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("not a PNG")

    width = height = None
    idat = bytearray()
    pos = 8
    while pos < len(blob):
        (length,) = struct.unpack(">I", blob[pos : pos + 4])
        kind = blob[pos + 4 : pos + 8]
        data = blob[pos + 8 : pos + 8 + length]
        pos += 12 + length  # length + type + data + crc

        if kind == b"IHDR":
            width, height, depth, color, _, _, interlace = struct.unpack(">IIBBBBB", data)
            if depth != 8 or color != 6:
                raise ValueError(f"need 8-bit RGBA (depth={depth} colortype={color})")
            if interlace:
                raise ValueError("interlaced PNG not supported")
        elif kind == b"IDAT":
            idat += data
        elif kind == b"IEND":
            break

    if width is None:
        raise ValueError("no IHDR")

    raw = zlib.decompress(bytes(idat))
    stride = width * 4
    out = bytearray(height * stride)

    # Undo the per-scanline filters (PNG spec §9.2). `prior` is the already
    # reconstructed row above; bpp is 4 for RGBA8.
    bpp = 4
    src = 0
    for y in range(height):
        ftype = raw[src]
        src += 1
        line = bytearray(raw[src : src + stride])
        src += stride
        base = y * stride
        prior = out[base - stride : base] if y else bytes(stride)

        if ftype == 0:
            pass
        elif ftype == 1:  # Sub
            for i in range(bpp, stride):
                line[i] = (line[i] + line[i - bpp]) & 0xFF
        elif ftype == 2:  # Up
            for i in range(stride):
                line[i] = (line[i] + prior[i]) & 0xFF
        elif ftype == 3:  # Average
            for i in range(stride):
                left = line[i - bpp] if i >= bpp else 0
                line[i] = (line[i] + ((left + prior[i]) >> 1)) & 0xFF
        elif ftype == 4:  # Paeth
            for i in range(stride):
                a = line[i - bpp] if i >= bpp else 0
                b = prior[i]
                c = prior[i - bpp] if i >= bpp else 0
                p = a + b - c
                pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
                pred = a if (pa <= pb and pa <= pc) else (b if pb <= pc else c)
                line[i] = (line[i] + pred) & 0xFF
        else:
            raise ValueError(f"bad filter type {ftype} on row {y}")

        out[base : base + stride] = line

    return width, height, bytes(out)


def resize(src, sw, sh, dw, dh):
    """Area-average downscale of RGBA bytes, premultiplying so edges don't halo.

    Averaging straight (non-premultiplied) RGBA pulls the colour of fully
    transparent pixels into the result — on a rounded mark that fringes the
    corners. Weighting colour by alpha and dividing back out at the end is the
    correct way round.
    """
    dst = bytearray(dw * dh * 4)
    for y in range(dh):
        y0, y1 = y * sh // dh, max(y * sh // dh + 1, (y + 1) * sh // dh)
        for x in range(dw):
            x0, x1 = x * sw // dw, max(x * sw // dw + 1, (x + 1) * sw // dw)
            r = g = b = a = n = 0
            for sy in range(y0, y1):
                row = sy * sw * 4
                for sx in range(x0, x1):
                    i = row + sx * 4
                    pa = src[i + 3]
                    r += src[i] * pa
                    g += src[i + 1] * pa
                    b += src[i + 2] * pa
                    a += pa
                    n += 1
            o = (y * dw + x) * 4
            if a:
                dst[o] = min(255, r // a)
                dst[o + 1] = min(255, g // a)
                dst[o + 2] = min(255, b // a)
            dst[o + 3] = a // n
    return bytes(dst)


def encode_png(rgba, w, h):
    def chunk(kind, data):
        c = kind + data
        return struct.pack(">I", len(data)) + c + struct.pack(">I", zlib.crc32(c))

    raw = bytearray()
    for y in range(h):
        raw.append(0)  # filter: none — zlib does fine on artwork this small
        raw += rgba[y * w * 4 : (y + 1) * w * 4]

    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(bytes(raw), 9))
        + chunk(b"IEND", b"")
    )


def encode_dib(rgba, w, h):
    """32-bit BITMAPINFOHEADER DIB: bottom-up BGRA, then the legacy AND mask.

    biHeight is doubled because the format expects colour and mask stacked. The
    mask is all-zero (nothing masked) — every consumer that matters reads the
    alpha channel instead — but it must still be present and 4-byte aligned per
    row or the entry is rejected.
    """
    header = struct.pack("<IiiHHIIiiII", 40, w, h * 2, 1, 32, 0, w * h * 4, 0, 0, 0, 0)

    xor = bytearray()
    for y in range(h - 1, -1, -1):
        row = y * w * 4
        for x in range(w):
            i = row + x * 4
            xor += bytes((rgba[i + 2], rgba[i + 1], rgba[i], rgba[i + 3]))

    mask_stride = ((w + 31) // 32) * 4
    return header + bytes(xor) + bytes(mask_stride * h)


def main():
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    src_path = os.path.join(root, "assets", "icon-rounded-1024.png")
    out_path = os.path.join(root, "windows", "Abacad.ico")

    w, h, rgba = decode_png(open(src_path, "rb").read())
    print(f"source {os.path.relpath(src_path, root)} {w}x{h}")

    images = []
    for size in SIZES:
        scaled = rgba if size == w else resize(rgba, w, h, size, size)
        blob = encode_png(scaled, size, size) if size > PNG_ABOVE else encode_dib(scaled, size, size)
        images.append((size, blob))
        print(f"  {size:>3}px  {'png' if size > PNG_ABOVE else 'dib'}  {len(blob):>7} bytes")

    # ICONDIR, then one 16-byte ICONDIRENTRY each, then the payloads. A 256px
    # entry writes its dimension as 0 — the byte only holds 0..255.
    offset = 6 + 16 * len(images)
    directory = struct.pack("<HHH", 0, 1, len(images))
    for size, blob in images:
        edge = 0 if size == 256 else size
        directory += struct.pack("<BBBBHHII", edge, edge, 0, 0, 1, 32, len(blob), offset)
        offset += len(blob)

    ico = directory + b"".join(blob for _, blob in images)
    with open(out_path, "wb") as fh:
        fh.write(ico)
    print(f"wrote {os.path.relpath(out_path, root)} ({len(ico)} bytes, {len(images)} frames)")


if __name__ == "__main__":
    sys.exit(main())
