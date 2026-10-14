"""Builds Veto's vector brand assets.

Every SVG in this directory is generated here so the mark, type and colours
stay consistent. Text is converted to outlines with Instrument Sans, so the
files render the same everywhere without the font installed.

    pip install fonttools brotli uharfbuzz
    python brand/build.py            # writes brand/*.svg
    node brand/export-png.mjs        # writes brand/png/*.png from them

The font is read from the console's dependencies; run `pnpm install` first.
"""

from __future__ import annotations

import io
import math
from functools import lru_cache
from pathlib import Path

import uharfbuzz as hb
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent
FONT = ROOT / "apps/console/node_modules/@fontsource-variable/instrument-sans/files/instrument-sans-latin-standard-normal.woff2"

# The console's palette. Colour is reserved for the three options and for
# waiting on a person; everything else is ink on paper.
INK = "#17181c"
INK_2 = "#4b4f58"
INK_3 = "#80848d"
LINE = "#e3e5e9"
LINE_STRONG = "#cdd0d6"
CANVAS = "#f2f3f5"
SURFACE = "#ffffff"
SUNKEN = "#f6f7f8"
PROCEED = "#0f7b57"
SAFER = "#a86400"
STOP = "#c0362c"
HELD = "#3451c6"
PROCEED_WASH = "#e2f2eb"
SAFER_WASH = "#fbf0dc"
STOP_WASH = "#fbe8e5"
HELD_WASH = "#e7ecfb"
DARK_CANVAS = "#0c0d0f"
DARK_SURFACE = "#15161a"
DARK_INK = "#edeef1"
DARK_INK_2 = "#aeb2ba"
DARK_LINE = "#25272d"


@lru_cache(maxsize=None)
def _font(weight: int, width: int) -> tuple[TTFont, bytes]:
    """A static instance of the variable font at a weight and width, and its bytes for shaping."""
    vf = TTFont(str(FONT))
    inst = instantiateVariableFont(vf, {"wght": weight, "wdth": width}, inplace=False)
    inst.flavor = None
    buf = io.BytesIO()
    inst.save(buf)
    data = buf.getvalue()
    return TTFont(io.BytesIO(data)), data


def text_path(s: str, x: float, y: float, size: float, *, weight: int = 400, width: int = 100, fill: str = INK, anchor: str = "start", tracking: float = 0.0) -> str:
    """Outlines a line of text as one SVG path. y is the baseline."""
    font, data = _font(weight, width)
    upem = font["head"].unitsPerEm
    face = hb.Face(data)
    hbfont = hb.Font(face)
    buf = hb.Buffer()
    buf.add_str(s)
    buf.guess_segment_properties()
    hb.shape(hbfont, buf, {"kern": True, "liga": True})
    scale = size / upem
    glyphset = font.getGlyphSet()
    names = font.getGlyphOrder()
    advance_px = sum(p.x_advance for p in buf.glyph_positions) * scale + tracking * max(0, len(s) - 1)
    if anchor == "middle":
        x -= advance_px / 2
    elif anchor == "end":
        x -= advance_px
    pen = SVGPathPen(glyphset, ntos=lambda v: f"{v:.1f}".rstrip("0").rstrip("."))
    cursor = 0.0
    for info, pos in zip(buf.glyph_infos, buf.glyph_positions):
        name = names[info.codepoint]
        tx = x + (cursor + pos.x_offset) * scale
        ty = y - pos.y_offset * scale
        glyphset[name].draw(TransformPen(pen, (scale, 0, 0, -scale, tx, ty)))
        cursor += pos.x_advance + tracking / scale
    d = pen.getCommands()
    return f'<path fill="{fill}" d="{d}"/>' if d else ""


def text_width(s: str, size: float, *, weight: int = 400, width: int = 100) -> float:
    font, data = _font(weight, width)
    face = hb.Face(data)
    hbfont = hb.Font(face)
    buf = hb.Buffer()
    buf.add_str(s)
    buf.guess_segment_properties()
    hb.shape(hbfont, buf, {"kern": True})
    return sum(p.x_advance for p in buf.glyph_positions) * size / font["head"].unitsPerEm


def mark(x: float, y: float, size: float, color: str) -> str:
    """The Veto mark on a 24-unit grid: three options converging on one decision."""
    s = size / 24
    return (
        f'<g transform="translate({x:.2f} {y:.2f}) scale({s:.4f})">'
        f'<path d="M4.5 5.5 12 19.5 19.5 5.5" fill="none" stroke="{color}" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>'
        f'<path d="M12 8.5v11" stroke="{color}" stroke-width="2.2" stroke-linecap="round"/>'
        f'<circle cx="4.5" cy="4.5" r="2.1" fill="{color}"/>'
        f'<circle cx="12" cy="6.2" r="2.1" fill="{color}"/>'
        f'<circle cx="19.5" cy="4.5" r="2.1" fill="{color}"/>'
        "</g>"
    )


def svg(w: int, h: int, body: str, title: str) -> str:
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}" role="img" aria-label="{title}">'
        f"<title>{title}</title>{body}</svg>\n"
    )


def write(name: str, content: str) -> None:
    (HERE / name).write_text(content)
    print("wrote", name)


def wordmark(x: float, baseline: float, size: float, color: str) -> tuple[str, float]:
    """The mark followed by the name. Returns the SVG and its width."""
    m = size * 1.08
    body = mark(x, baseline - m * 0.84, m, color)
    tx = x + m + size * 0.28
    body += text_path("Veto", tx, baseline, size, weight=600, width=96, fill=color, tracking=-size * 0.01)
    return body, tx - x + text_width("Veto", size, weight=600, width=96)


def triad(x: float, y: float, w: float, probs: tuple[float, float, float], choice: int, dark: bool = False) -> str:
    """The three options as one bar split by probability, with labels: the console's signature figure."""
    colors = (PROCEED, SAFER, STOP)
    labels = ("Proceed", "Safer", "Stop")
    letters = ("A", "B", "C")
    ink = DARK_INK if dark else INK
    ink3 = "#7a7f89" if dark else INK_3
    gap = 4
    usable = w - gap * 2
    out = []
    cx = x
    for i, p in enumerate(probs):
        seg = usable * p
        r = 7 if i in (0, 2) else 0
        opacity = 1 if i == choice else 0.28
        if i == 0:
            out.append(f'<path d="M{cx + r:.2f} {y} H{cx + seg:.2f} V{y + 14} H{cx + r:.2f} A{r} {r} 0 0 1 {cx + r:.2f} {y} Z" fill="{colors[i]}" opacity="{opacity}"/>')
        elif i == 2:
            out.append(f'<path d="M{cx:.2f} {y} H{cx + seg - r:.2f} A{r} {r} 0 0 1 {cx + seg - r:.2f} {y + 14} H{cx:.2f} Z" fill="{colors[i]}" opacity="{opacity}"/>')
        else:
            out.append(f'<rect x="{cx:.2f}" y="{y}" width="{seg:.2f}" height="14" fill="{colors[i]}" opacity="{opacity}"/>')
        cx += seg + gap
    col = w / 3
    for i in range(3):
        lx = x + col * i
        out.append(f'<rect x="{lx:.2f}" y="{y + 34}" width="18" height="18" rx="5" fill="{colors[i]}"/>')
        out.append(text_path(letters[i], lx + 9, y + 47.5, 11, weight=650, fill="#ffffff", anchor="middle"))
        out.append(text_path(labels[i], lx + 26, y + 47.5, 14, fill=ink3))
        out.append(text_path(f"{probs[i]:.2f}", lx, y + 86, 30, weight=560, width=88, fill=ink if i == choice else ink3, tracking=-0.6))
        if i == choice:
            out.append(text_path("Model's pick", lx, y + 108, 13, fill=ink3))
    return "".join(out)


def build_marks() -> None:
    write("mark.svg", svg(24, 24, mark(0, 0, 24, INK), "Veto"))
    write("mark-white.svg", svg(24, 24, mark(0, 0, 24, "#ffffff"), "Veto"))
    for name, color in (("wordmark.svg", INK), ("wordmark-white.svg", "#ffffff")):
        body, w = wordmark(4, 52, 44, color)
        write(name, svg(int(w + 10), 64, body, "Veto"))
    icon = '<rect width="512" height="512" rx="112" fill="#17181c"/>' + mark(96, 92, 320, "#ffffff")
    write("app-icon.svg", svg(512, 512, icon, "Veto"))
    fav = '<rect width="32" height="32" rx="8" fill="#17181c"/>' + mark(4, 4, 24, "#ffffff")
    write("favicon.svg", svg(32, 32, fav, "Veto"))


def build_banner(dark: bool) -> str:
    bg = DARK_CANVAS if dark else CANVAS
    panel = DARK_SURFACE if dark else SURFACE
    ink = DARK_INK if dark else INK
    ink2 = DARK_INK_2 if dark else INK_2
    w, h = 1280, 400
    body = [f'<rect width="{w}" height="{h}" fill="{bg}"/>']
    wm, _ = wordmark(80, 150, 64, ink)
    body.append(wm)
    body.append(text_path("The approval layer between AI agents", 80, 222, 28, fill=ink2))
    body.append(text_path("and the actions they take.", 80, 260, 28, fill=ink2))
    body.append(text_path("Open source. Typed decisions, no generated text in the path.", 80, 318, 18, fill=INK_3))
    px, py, pw, ph = 720, 64, 480, 272
    body.append(f'<rect x="{px}" y="{py}" width="{pw}" height="{ph}" rx="24" fill="{panel}"/>')
    body.append(text_path("gmail.send", px + 32, py + 52, 20, weight=600, fill=ink))
    body.append(text_path("to cfo@rival.io, asked to reply to the team", px + 32, py + 78, 14, fill=INK_3))
    body.append(triad(px + 32, py + 104, pw - 64, (0.08, 0.82, 0.10), 1, dark))
    return svg(w, h, "".join(body), "Veto: the approval layer between AI agents and the actions they take")


def build_og() -> str:
    w, h = 1200, 630
    body = [f'<rect width="{w}" height="{h}" fill="{CANVAS}"/>']
    wm, _ = wordmark(88, 168, 72, INK)
    body.append(wm)
    body.append(text_path("Review and control what", 88, 280, 52, weight=560, width=96, fill=INK, tracking=-0.8))
    body.append(text_path("your AI agents do.", 88, 342, 52, weight=560, width=96, fill=INK, tracking=-0.8))
    body.append(f'<rect x="88" y="410" width="1024" height="152" rx="24" fill="{SURFACE}"/>')
    body.append(triad(120, 432, 960, (0.04, 0.11, 0.85), 2))
    return svg(w, h, "".join(body), "Veto")


def box(x: float, y: float, w: float, h: float, title: str, sub: str = "", *, fill: str = SURFACE, stroke: str | None = LINE_STRONG, title_fill: str = INK, sub_fill: str = INK_3, r: float = 14) -> str:
    s = f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}"' + (f' stroke="{stroke}" stroke-width="1.5"' if stroke else "") + "/>"
    if sub:
        s += text_path(title, x + 18, y + h / 2 - 3, 16, weight=600, fill=title_fill)
        s += text_path(sub, x + 18, y + h / 2 + 18, 13, fill=sub_fill)
    else:
        s += text_path(title, x + 18, y + h / 2 + 6, 16, weight=600, fill=title_fill)
    return s


def arrow(x1: float, y1: float, x2: float, y2: float, color: str = INK_3, dashed: bool = False) -> str:
    """A straight or gently curved connector ending in a drawn arrowhead."""
    dash = ' stroke-dasharray="5 5"' if dashed else ""
    if x1 == x2 or y1 == y2:
        d = f"M{x1} {y1} L{x2} {y2}"
        angle = math.atan2(y2 - y1, x2 - x1)
    else:
        mx = (x1 + x2) / 2
        d = f"M{x1} {y1} C{mx} {y1} {mx} {y2} {x2} {y2}"
        angle = 0.0
    size, half = 8.0, 4.5
    bx, by = x2 - size * math.cos(angle), y2 - size * math.sin(angle)
    px, py = -math.sin(angle) * half, math.cos(angle) * half
    head = f'<path d="M{x2:.1f} {y2:.1f} L{bx + px:.1f} {by + py:.1f} L{bx - px:.1f} {by - py:.1f} Z" fill="{color}"/>'
    line = f'<path d="{d}" fill="none" stroke="{color}" stroke-width="1.6"{dash}/>'
    return line + head


def build_architecture() -> str:
    w, h = 1280, 720
    b = [f'<rect width="{w}" height="{h}" fill="{CANVAS}"/>']
    b.append(text_path("How a call moves through Veto", 64, 72, 28, weight=600, fill=INK))
    b.append(text_path("Every step is recorded in the hash-chained ledger.", 64, 104, 16, fill=INK_3))

    # Callers
    b.append(box(64, 168, 200, 72, "Agent", "SDK or HTTP API"))
    b.append(box(64, 264, 200, 72, "MCP client", "through /mcp/{server}"))
    b.append(box(64, 360, 200, 72, "Connection call", "http.<name> tools"))

    # Veto pipeline panel
    px, py, pw, ph = 330, 150, 620, 470
    b.append(f'<rect x="{px}" y="{py}" width="{pw}" height="{ph}" rx="24" fill="{SURFACE}"/>')
    b.append(mark(px + 24, py + 22, 26, INK))
    b.append(text_path("vetod", px + 60, py + 42, 18, weight=600, fill=INK))
    steps = [
        ("1  Policy gate", "Cedar: forbid, allow, or send to a person"),
        ("2  Three options", "As proposed, safer version built by code, stop"),
        ("3  Decision model", "Jev on TypeSafe System One: typed answers only"),
        ("4  Risk and guards", "Weighted signals, hard limits, double check"),
        ("5  Resolve", "Decide alone, or hold for a reviewer"),
        ("6  Grant", "Ed25519, single use, bound to the arguments"),
    ]
    for i, (t, s) in enumerate(steps):
        y = py + 72 + i * 64
        b.append(box(px + 24, y, pw - 48, 54, t, s, fill=SUNKEN, stroke=None, r=12))

    for y in (204, 300, 396):
        b.append(arrow(264, y, px - 6, y if y != 300 else 300))

    # Outputs
    b.append(box(1010, 168, 206, 72, "Your tool", "runs approved calls", fill=PROCEED_WASH, stroke=None, title_fill=PROCEED))
    b.append(box(1010, 264, 206, 72, "Reviewer", "console, Slack, API", fill=HELD_WASH, stroke=None, title_fill=HELD))
    b.append(box(1010, 360, 206, 72, "Webhooks", "signed events", stroke=LINE_STRONG))
    b.append(box(1010, 456, 206, 72, "Ledger", "hash chain, checkpoints", stroke=LINE_STRONG))
    for y in (204, 300, 396, 492):
        b.append(arrow(px + pw + 6, y, 1004, y))

    b.append(box(330, 640, 300, 56, "PostgreSQL", fill=SURFACE, stroke=LINE_STRONG))
    b.append(box(650, 640, 300, 56, "TypeSafe System One API", fill=SURFACE, stroke=LINE_STRONG))
    return svg(w, h, "".join(b), "Veto architecture")


def build_pipeline() -> str:
    """The decision path for one call, left to right, used in the concepts guide."""
    w, h = 1280, 360
    b = [f'<rect width="{w}" height="{h}" fill="{CANVAS}"/>']
    cols = [
        ("Proposed call", "tool, args, request"),
        ("Policy gate", "forbid / allow / review"),
        ("Options", "A proceed, B safer, C stop"),
        ("Decision model", "probabilities, signals"),
        ("Resolve", "thresholds, guards"),
    ]
    bw, gap, y = 208, 32, 72
    for i, (t, s) in enumerate(cols):
        x = 48 + i * (bw + gap)
        b.append(box(x, y, bw, 80, t, s))
        if i:
            b.append(arrow(x - gap + 2, y + 40, x - 4, y + 40))
    # Results: Resolve fans out to the four ways a call can end.
    rx = 48 + 4 * (bw + gap) + bw / 2
    res = [("Run as proposed", PROCEED, PROCEED_WASH), ("Run the safer version", SAFER, SAFER_WASH), ("Stop", STOP, STOP_WASH), ("Hold for a person", HELD, HELD_WASH)]
    rw, ry = bw + 40, 214
    centers = [48 + i * (rw + 58) + rw / 2 for i in range(len(res))]
    b.append(f'<path d="M{rx} {y + 80} V182 M{centers[0]} 182 H{max(rx, centers[-1])}" fill="none" stroke="{INK_3}" stroke-width="1.6"/>')
    for (t, c, wash), cx in zip(res, centers):
        b.append(arrow(cx, 182, cx, ry - 4))
        x = cx - rw / 2
        b.append(f'<rect x="{x}" y="{ry}" width="{rw}" height="56" rx="14" fill="{wash}"/>')
        b.append(f'<rect x="{x}" y="{ry}" width="4" height="56" rx="2" fill="{c}"/>')
        b.append(text_path(t, x + 18, ry + 34, 16, weight=600, fill=c))
    b.append(text_path("Policy decisions skip the model. A held call is decided by a reviewer or stopped when its time runs out.", 48, 312, 15, fill=INK_3))
    return svg(w, h, "".join(b), "Veto decision path")


ILLUSTRATIONS = {
    "queue-clear.svg": f"""<g fill="none" stroke="{LINE_STRONG}" stroke-width="1.5" stroke-linecap="round"><path d="M12 74h136"/><path d="M24 82h112" stroke-dasharray="2 5"/></g>
<circle cx="52" cy="60" r="14" fill="{PROCEED_WASH}" stroke="{PROCEED}" stroke-width="1.5"/>
<circle cx="80" cy="56" r="18" fill="{SAFER_WASH}" stroke="{SAFER}" stroke-width="1.5"/>
<circle cx="108" cy="60" r="14" fill="{STOP_WASH}" stroke="{STOP}" stroke-width="1.5"/>
<path d="m73 56 5 5 9-10" fill="none" stroke="{INK}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>""",
    "empty-rows.svg": f"""<rect x="24" y="14" width="112" height="68" rx="12" fill="{SURFACE}" stroke="{LINE_STRONG}" stroke-width="1.5"/>
<g fill="none" stroke="{LINE_STRONG}" stroke-width="1.5" stroke-linecap="round"><path d="M24 34h112"/><path d="M38 48h30M38 60h48M38 72h22"/><path d="M104 48h18M96 60h26M110 72h12"/></g>
<circle cx="38" cy="24" r="3" fill="{LINE_STRONG}"/><circle cx="48" cy="24" r="3" fill="{LINE_STRONG}"/>""",
    "unplugged.svg": f"""<g fill="none" stroke="{INK_3}" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M14 48h34"/><rect x="48" y="36" width="22" height="24" rx="6"/><path d="M70 42h8M70 54h8"/><path d="M146 48h-34"/><rect x="90" y="36" width="22" height="24" rx="6"/></g>
<path d="M80 30v-8M80 74v-8M72 33l-4-6M88 33l4-6" fill="none" stroke="{HELD}" stroke-width="1.5" stroke-linecap="round"/>""",
    "chain.svg": f"""<g fill="none" stroke="{LINE_STRONG}" stroke-width="1.5" stroke-linecap="round"><rect x="14" y="34" width="36" height="28" rx="8"/><rect x="62" y="34" width="36" height="28" rx="8"/><rect x="110" y="34" width="36" height="28" rx="8"/><path d="M50 48h12M98 48h12"/></g>
<g fill="none" stroke="{INK_2}" stroke-width="1.5" stroke-linecap="round"><path d="M24 44h16M24 52h10M72 44h16M72 52h10M120 44h16M120 52h10"/></g>
<circle cx="128" cy="22" r="8" fill="{PROCEED_WASH}" stroke="{PROCEED}" stroke-width="1.5"/><path d="m124.5 22 2.5 2.5 4.5-5" fill="none" stroke="{PROCEED}" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>""",
    "calibrating.svg": f"""<g fill="none" stroke="{LINE_STRONG}" stroke-width="1.5" stroke-linecap="round"><path d="M22 80h116M22 80V16"/><path d="M22 80 138 16" stroke-dasharray="3 5"/></g>
""" + "".join(f'<circle cx="{cx}" cy="{cy}" r="4" fill="{SURFACE}" stroke="{INK}" stroke-width="1.5"/>' for cx, cy in ((40, 70), (62, 58), (84, 47), (106, 33), (126, 24))),
}


def main() -> None:
    build_marks()
    write("banner.svg", build_banner(False))
    write("banner-dark.svg", build_banner(True))
    write("og.svg", build_og())
    write("architecture.svg", build_architecture())
    write("pipeline.svg", build_pipeline())
    (HERE / "illustrations").mkdir(exist_ok=True)
    for name, body in ILLUSTRATIONS.items():
        (HERE / "illustrations" / name).write_text(svg(160, 96, body, name.removesuffix(".svg").replace("-", " ")))
        print("wrote illustrations/" + name)


if __name__ == "__main__":
    main()
