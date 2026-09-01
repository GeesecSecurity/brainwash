#!/usr/bin/env python3
"""Fit artwork to the macOS Dock/Finder icon grid.

System app icons occupy ~83% of the 1024 canvas (about 848px, ~88px
margin). A full-bleed squircle looks a size class larger in the Dock.
"""

from __future__ import annotations

import struct
import sys
import zlib
from pathlib import Path

# Apple's production grid for a 1024 macOS app icon.
CANVAS = 1024
CONTENT = 848  # 848/1024 == 0.828, matches Calculator/Notes/Terminal
SQUIRCLE_N = 5.0


def read_png(path: Path) -> tuple[int, int, list[bytearray]]:
    data = path.read_bytes()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise SystemExit(f"not a png: {path}")
    off = 8
    w = h = bit = color = None
    idat = b""
    while off < len(data):
        ln = struct.unpack(">I", data[off : off + 4])[0]
        typ = data[off + 4 : off + 8]
        chunk = data[off + 8 : off + 8 + ln]
        off += 12 + ln
        if typ == b"IHDR":
            w, h, bit, color, _, _, _ = struct.unpack(">IIBBBBB", chunk)
        elif typ == b"IDAT":
            idat += chunk
        elif typ == b"IEND":
            break
    if None in (w, h) or bit != 8 or color != 6:
        raise SystemExit(f"need 8-bit RGBA png, got {w}x{h} bit={bit} color={color}")
    raw = zlib.decompress(idat)
    bpp, stride, i = 4, w * 4, 0
    prev = bytearray(stride)
    rows: list[bytearray] = []
    for _ in range(h):
        f = raw[i]
        i += 1
        row = bytearray(raw[i : i + stride])
        i += stride
        if f == 1:
            for x in range(stride):
                left = row[x - bpp] if x >= bpp else 0
                row[x] = (row[x] + left) & 255
        elif f == 2:
            for x in range(stride):
                row[x] = (row[x] + prev[x]) & 255
        elif f == 3:
            for x in range(stride):
                left = row[x - bpp] if x >= bpp else 0
                row[x] = (row[x] + ((left + prev[x]) // 2)) & 255
        elif f == 4:
            for x in range(stride):
                a = row[x - bpp] if x >= bpp else 0
                b = prev[x]
                c = prev[x - bpp] if x >= bpp else 0
                p = a + b - c
                pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
                pr = a if pa <= pb and pa <= pc else (b if pb <= pc else c)
                row[x] = (row[x] + pr) & 255
        elif f != 0:
            raise SystemExit(f"unsupported png filter {f}")
        rows.append(row)
        prev = row
    return w, h, rows


def write_png(path: Path, w: int, h: int, rows: list[bytearray]) -> None:
    raw = bytearray()
    for row in rows:
        raw.append(0)
        raw.extend(row)

    def chunk(typ: bytes, payload: bytes) -> bytes:
        crc = zlib.crc32(typ)
        crc = zlib.crc32(payload, crc) & 0xFFFFFFFF
        return struct.pack(">I", len(payload)) + typ + payload + struct.pack(">I", crc)

    ihdr = struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0)
    idat = zlib.compress(bytes(raw), 9)
    path.write_bytes(
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", ihdr)
        + chunk(b"IDAT", idat)
        + chunk(b"IEND", b"")
    )


def sample(rows: list[bytearray], w: int, h: int, x: float, y: float) -> tuple[int, int, int, int]:
    if x < 0 or y < 0 or x >= w - 1 or y >= h - 1:
        x = min(max(x, 0.0), w - 1.001)
        y = min(max(y, 0.0), h - 1.001)
    x0 = int(x)
    y0 = int(y)
    x1 = min(x0 + 1, w - 1)
    y1 = min(y0 + 1, h - 1)
    tx = x - x0
    ty = y - y0
    out = [0, 0, 0, 0]
    for i in range(4):
        v00 = rows[y0][x0 * 4 + i]
        v10 = rows[y0][x1 * 4 + i]
        v01 = rows[y1][x0 * 4 + i]
        v11 = rows[y1][x1 * 4 + i]
        v0 = v00 * (1 - tx) + v10 * tx
        v1 = v01 * (1 - tx) + v11 * tx
        out[i] = int(v0 * (1 - ty) + v1 * ty + 0.5)
    return out[0], out[1], out[2], out[3]


def squircle_coverage(x: float, y: float, cx: float, cy: float, r: float, n: float) -> float:
    acc = 0.0
    for dx, dy in ((-0.25, -0.25), (0.25, -0.25), (-0.25, 0.25), (0.25, 0.25)):
        nx = abs((x + dx - cx) / r)
        ny = abs((y + dy - cy) / r)
        acc += 1.0 if (nx**n + ny**n) <= 1.0 else 0.0
    return acc / 4.0


def compose(src_w: int, src_h: int, src: list[bytearray]) -> list[bytearray]:
    canvas = [bytearray(CANVAS * 4) for _ in range(CANVAS)]
    origin = (CANVAS - CONTENT) // 2
    r = (CONTENT - 1) / 2.0
    cx = origin + r
    cy = origin + r
    scale_x = src_w / CONTENT
    scale_y = src_h / CONTENT

    # Soft drop shadow, similar to system app icons.
    shadow = [bytearray(CANVAS) for _ in range(CANVAS)]
    for y in range(origin, origin + CONTENT):
        for x in range(origin, origin + CONTENT):
            cov = squircle_coverage(x, y, cx, cy, r, SQUIRCLE_N)
            if cov > 0:
                shadow[y][x] = int(90 * cov)
    shadow = box_blur(shadow, CANVAS, CANVAS, radius=14)
    y_off = 10
    for y in range(CANVAS):
        sy = y - y_off
        if sy < 0 or sy >= CANVAS:
            continue
        row = canvas[y]
        sh = shadow[sy]
        for x in range(CANVAS):
            a = sh[x]
            if a:
                row[x * 4 + 3] = a

    for y in range(origin, origin + CONTENT):
        sy = (y - origin + 0.5) * scale_y - 0.5
        row = canvas[y]
        for x in range(origin, origin + CONTENT):
            cov = squircle_coverage(x, y, cx, cy, r, SQUIRCLE_N)
            if cov <= 0:
                continue
            sx = (x - origin + 0.5) * scale_x - 0.5
            sr, sg, sb, sa = sample(src, src_w, src_h, sx, sy)
            sa = int(sa * cov)
            if sa <= 0:
                continue
            di = x * 4
            dr, dg, db, da = row[di], row[di + 1], row[di + 2], row[di + 3]
            out_a = sa + da * (255 - sa) // 255
            if out_a == 0:
                continue
            row[di] = (sr * sa + dr * da * (255 - sa) // 255) // out_a
            row[di + 1] = (sg * sa + dg * da * (255 - sa) // 255) // out_a
            row[di + 2] = (sb * sa + db * da * (255 - sa) // 255) // out_a
            row[di + 3] = out_a
    return canvas


def box_blur(src: list[bytearray], w: int, h: int, radius: int) -> list[bytearray]:
    if radius <= 0:
        return src
    tmp = [bytearray(w) for _ in range(h)]
    out = [bytearray(w) for _ in range(h)]
    span = radius * 2 + 1
    for y in range(h):
        acc = 0
        row = src[y]
        dst = tmp[y]
        for x in range(-radius, radius + 1):
            acc += row[0] if x < 0 else row[x if x < w else w - 1]
        for x in range(w):
            dst[x] = acc // span
            left = x - radius
            right = x + radius + 1
            acc -= row[0] if left < 0 else row[left]
            acc += row[w - 1] if right >= w else row[right]
    for x in range(w):
        acc = 0
        for y in range(-radius, radius + 1):
            acc += tmp[0][x] if y < 0 else tmp[y if y < h else h - 1][x]
        for y in range(h):
            out[y][x] = acc // span
            top = y - radius
            bot = y + radius + 1
            acc -= tmp[0][x] if top < 0 else tmp[top][x]
            acc += tmp[h - 1][x] if bot >= h else tmp[bot][x]
    return out


def main() -> None:
    src = Path(sys.argv[1])
    dst = Path(sys.argv[2])
    w, h, rows = read_png(src)
    canvas = compose(w, h, rows)
    write_png(dst, CANVAS, CANVAS, canvas)
    print(dst)


if __name__ == "__main__":
    main()
