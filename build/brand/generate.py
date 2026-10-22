#!/usr/bin/env python3
"""Generate committed SADPTools branding from mark.svg (Linux authoring tool).

Requires Python 3.11+, librsvg 2.46+, Cairo and DejaVu Sans. Normal application
builds use the committed assets and do not need these rendering libraries.
"""
import argparse
import ctypes as c
import ctypes.util
from pathlib import Path
import struct
import tempfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
BRAND = Path(__file__).parent
INK = "#101E2B"
TEAL = "#36D6C7"
AMBER = "#F5B84B"
WHITE = "#F1F6FA"
SIZES = (16, 20, 24, 32, 40, 48, 64, 128, 256)


class Rect(c.Structure):
    _fields_ = [(name, c.c_double) for name in ("x", "y", "width", "height")]


class Renderer:
    def __init__(self):
        def library(name):
            path = ctypes.util.find_library(name)
            if not path:
                raise RuntimeError(f"Missing {name}; see docs/branding.md")
            return c.CDLL(path)

        self.svg = library("rsvg-2")
        self.cairo = library("cairo")
        self.obj = library("gobject-2.0")
        for lib, name, args, result in (
            (self.svg, "rsvg_handle_new_from_data", [c.c_char_p, c.c_size_t, c.c_void_p], c.c_void_p),
            (self.svg, "rsvg_handle_render_document", [c.c_void_p, c.c_void_p, c.POINTER(Rect), c.c_void_p], c.c_int),
            (self.cairo, "cairo_image_surface_create", [c.c_int, c.c_int, c.c_int], c.c_void_p),
            (self.cairo, "cairo_create", [c.c_void_p], c.c_void_p),
            (self.cairo, "cairo_status", [c.c_void_p], c.c_int),
            (self.cairo, "cairo_surface_write_to_png", [c.c_void_p, c.c_char_p], c.c_int),
            (self.cairo, "cairo_surface_flush", [c.c_void_p], None),
            (self.cairo, "cairo_image_surface_get_data", [c.c_void_p], c.c_void_p),
            (self.cairo, "cairo_image_surface_get_stride", [c.c_void_p], c.c_int),
            (self.cairo, "cairo_destroy", [c.c_void_p], None),
            (self.cairo, "cairo_surface_destroy", [c.c_void_p], None),
            (self.obj, "g_object_unref", [c.c_void_p], None),
        ):
            fn = getattr(lib, name)
            fn.argtypes, fn.restype = args, result

    def render(self, source, width, height, bitmap=False):
        data = source.encode()
        handle = self.svg.rsvg_handle_new_from_data(data, len(data), None)
        if not handle:
            raise RuntimeError("Invalid brand SVG")
        surface = self.cairo.cairo_image_surface_create(0, width, height)
        context = self.cairo.cairo_create(surface)
        try:
            viewport = Rect(0, 0, width, height)
            if not self.svg.rsvg_handle_render_document(handle, context, c.byref(viewport), None) or self.cairo.cairo_status(context):
                raise RuntimeError("Could not render brand SVG")
            if bitmap:
                # Cairo ARGB32 is native-endian, premultiplied. Installer art is
                # opaque; emit a bottom-up 24-bit Windows BMP without alpha.
                self.cairo.cairo_surface_flush(surface)
                stride = self.cairo.cairo_image_surface_get_stride(surface)
                pixels = c.string_at(self.cairo.cairo_image_surface_get_data(surface), stride * height)
                rows = bytearray()
                for y in reversed(range(height)):
                    for x in range(width):
                        argb = struct.unpack_from("=I", pixels, y * stride + x * 4)[0]
                        if argb >> 24 != 255:
                            raise RuntimeError("Installer bitmap must be opaque")
                        rows.extend((argb & 255, (argb >> 8) & 255, (argb >> 16) & 255))
                    rows.extend(bytes((-width * 3) % 4))
                return struct.pack("<2sIHHI", b"BM", 54 + len(rows), 0, 0, 54) + struct.pack("<IiiHHIIiiII", 40, width, height, 1, 24, 0, len(rows), 2835, 2835, 0, 0) + rows
            with tempfile.TemporaryDirectory(prefix="sadptools-brand-") as directory:
                output = Path(directory) / "render.png"
                if self.cairo.cairo_surface_write_to_png(surface, str(output).encode()):
                    raise RuntimeError("Could not write brand PNG")
                return output.read_bytes()
        finally:
            self.cairo.cairo_destroy(context)
            self.cairo.cairo_surface_destroy(surface)
            self.obj.g_object_unref(handle)


def svg(width, height, body, title="SADPTools"):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}"><title>{title}</title>{body}</svg>\n'


def group(body, x, y, scale):
    return f'<g transform="translate({x} {y}) scale({scale})">{body}</g>'


def text(x, y, size, value, fill=WHITE, weight=400, spacing=0):
    return f'<text x="{x}" y="{y}" font-family="DejaVu Sans, sans-serif" font-size="{size}" font-weight="{weight}" letter-spacing="{spacing}" fill="{fill}">{value}</text>'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="Fail if committed exports differ from generated artwork")
    args = parser.parse_args()
    ET.register_namespace("", "http://www.w3.org/2000/svg")
    mark = "".join(ET.tostring(child, encoding="unicode") for child in ET.parse(BRAND / "mark.svg").getroot() if child.tag.rsplit("}", 1)[-1] not in ("title", "desc"))
    # Include fill explicitly because the source root's attributes are not copied.
    mark = f'<g fill="none">{mark}</g>'
    tile = f'<rect x="4" y="4" width="248" height="248" rx="54" fill="{INK}" stroke="#2D4656" stroke-width="2"/>{mark}'
    icon = svg(256, 256, tile, "SADPTools — Signal Lens")
    outputs = {
        "build/appicon.svg": icon.encode(),
        "frontend/src/assets/brand-icon.svg": icon.encode(),
    }
    for theme, colour, accent in (("dark", WHITE, TEAL), ("light", INK, "#08796F")):
        logo = svg(1000, 256, tile + text(302, 145, 94, 'SADP<tspan font-weight="400" fill="' + accent + '">Tools</tspan>', colour, 700, -3) + text(307, 193, 18, "NETWORK DEVICE MANAGEMENT", colour, 400, 3))
        outputs[f"build/brand/logo-{theme}.svg"] = logo.encode()
    sidebar = svg(328, 628, f'<rect width="328" height="628" fill="{INK}"/>' + group(mark, 36, 56, 1) + text(32, 369, 38, 'SADP<tspan fill="' + TEAL + '" font-weight="400">Tools</tspan>', WHITE, 700, -1) + text(34, 402, 13, "NETWORK DEVICE", "#A9BCC9", 400, 2) + text(34, 424, 13, "MANAGEMENT", "#A9BCC9", 400, 2) + f'<path d="M34 477H200L234 511H294" fill="none" stroke="#284554" stroke-width="2"/><circle cx="294" cy="511" r="5" fill="{AMBER}"/>' + text(34, 564, 14, "Created by Paul Willard", WHITE) + text(34, 589, 12, "TerminalAddict · New Zealand", "#A9BCC9"))
    header = svg(300, 114, '<rect width="300" height="114" fill="#FFFFFF"/>' + group(tile, 9, 13, 0.34) + text(111, 56, 28, 'SADP<tspan fill="#08796F" font-weight="400">Tools</tspan>', INK, 700, -1) + text(113, 79, 12, "by Paul Willard", "#526673"))
    outputs["build/brand/installer-sidebar.svg"] = sidebar.encode()
    outputs["build/brand/installer-header.svg"] = header.encode()
    renderer = Renderer()
    outputs["build/appicon.png"] = renderer.render(icon, 512, 512)
    frames = [renderer.render(icon, size, size) for size in SIZES]
    offset = 6 + 16 * len(frames)
    directory = bytearray(struct.pack("<HHH", 0, 1, len(frames)))
    for size, frame in zip(SIZES, frames):
        directory.extend(struct.pack("<BBBBHHII", size % 256, size % 256, 0, 0, 1, 32, len(frame), offset))
        offset += len(frame)
    outputs["build/windows/icon.ico"] = directory + b"".join(frames)
    outputs["build/windows/installer-sidebar.bmp"] = renderer.render(sidebar, 328, 628, True)
    outputs["build/windows/installer-header.bmp"] = renderer.render(header, 300, 114, True)
    board = '<rect width="1200" height="800" fill="#101E2B"/>'
    board += text(48, 58, 16, "SADPTOOLS / SIGNAL LENS", "#A9BCC9", 600, 3)
    board += group(tile, 48, 100, 1.1)
    board += text(386, 192, 70, 'SADP<tspan fill="' + TEAL + '" font-weight="400">Tools</tspan>', WHITE, 700, -2)
    board += text(389, 229, 15, "NETWORK DEVICE MANAGEMENT", "#A9BCC9", 400, 3)
    board += text(389, 293, 18, "A lens. A local network. A discovered device.", "#A9BCC9")
    board += '<rect x="48" y="426" width="1104" height="164" rx="16" fill="#F1F6FA"/>'
    board += text(72, 457, 12, "WINDOWS ICON SIZES / ACTUAL PIXELS", "#526673", 600, 2)
    for size, x in ((16, 80), (24, 144), (32, 218), (48, 307), (64, 418), (128, 570)):
        if size < 128:
            board += group(tile, x, 476, size / 256) + text(x, 571, 11, str(size), "#526673")
        else:
            board += group(tile, x, 446, 0.5) + text(722, 507, 16, "One mark, from toolbar to installer.", INK)
    for name, colour, x in (("MIDNIGHT", INK, 48), ("SIGNAL", TEAL, 330), ("DISCOVERY", AMBER, 612), ("FROST", WHITE, 894)):
        board += f'<rect x="{x}" y="634" width="258" height="58" rx="8" fill="{colour}" stroke="#2D4656"/>' + text(x, 721, 12, name, "#A9BCC9", 600, 2) + text(x, 746, 13, colour, WHITE)
    outputs["build/brand/preview.svg"] = svg(1200, 800, board).encode()
    outputs["build/brand/preview.png"] = renderer.render(svg(1200, 800, board), 1200, 800)
    different = []
    for name, data in outputs.items():
        destination = ROOT / name
        if args.check:
            if not destination.is_file() or destination.read_bytes() != data:
                different.append(name)
        else:
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(data)
    if different:
        raise SystemExit("Brand exports need regeneration: " + ", ".join(different))
    print(f"{'Verified' if args.check else 'Generated'} {len(outputs)} brand assets; ICO sizes: {', '.join(map(str, SIZES))}")


if __name__ == "__main__":
    main()
