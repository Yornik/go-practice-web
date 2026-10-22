# Makes the static TrueType fonts of the certificate PDF in internal/cert/fonts/, and measures them
# (.design/VESPER.md 13.1). The fonts stay committed; this script says where they come from and
# reproduces them byte for byte.
#
# Usage: python3 -I scripts/pdf-fonts.py [CACHE_DIR]
#   Needs fontTools 4.x. The first run downloads 3 fonts from google/fonts (about 650 KB) and the
#   Inter 4.1 release (about 33 MB) into CACHE_DIR (default tmp/font-src, which git ignores) and
#   checks each by sha256. Nothing in the cache is ever executed.
#
# Why static TrueType: gopdf v0.38.2 embeds glyf outlines and ignores variable axes, it reads only a
# (3,1) format 4 cmap, and it kerns only from a legacy 'kern' table (format 0, 1 subtable).
#
# Sources. The site ships @fontsource-variable 5.3.0, whose woff2 files are split by script (latin,
# latin-ext). A merge of 2 such subsets loses every kerning pair between them (Ł+u, o+ř, i+ś), so
# the PDF fonts start from the full variable TTFs those npm files were cut from: the same versions
# (Saira 1.101, Atkinson Hyperlegible Next and Mono 2.001), pinned to 1 google/fonts commit. The
# fallback for names is Inter Bold 4.1 (Version 4.001), from the rsms/inter release.
#
# What each output holds, and nothing more: the instance at the given axes; the code points of the
# site's subsets (the unicode-range lists of web/static/css/site.css); a 'kern' table flattened
# from the GPOS kern feature at that instance; no GSUB, GPOS or GDEF, because gopdf never shapes.
# Saira Kind also gets a slashed zero drawn into its 0, the instrument numeral of the site.
import hashlib
import math
import os
import re
import sys
import unicodedata
import urllib.request
import zipfile

from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.subset import Options, Subsetter
from fontTools.ttLib import TTFont, newTable
from fontTools.ttLib.tables._k_e_r_n import KernTable_format_0
from fontTools.varLib.instancer import OverlapMode, instantiateVariableFont

REPO = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
OUT = os.path.join(REPO, "internal", "cert", "fonts")
SITE_FONTS = os.path.join(REPO, "web", "static", "fonts")
CACHE = os.path.abspath(sys.argv[1]) if len(sys.argv) > 1 else os.path.join(REPO, "tmp", "font-src")

COMMIT = "45e3d5269e4a17d823f6d84470bc084dc5ae46f6"     # google/fonts main on 21 October 2026
SRC = {
    "saira": ("ofl/saira/Saira%5Bwdth,wght%5D.ttf",
              "9d050fc5a01c85f74c4257c207d50b55d1e40c37308c642f974a2c5003231dde"),
    "next": ("ofl/atkinsonhyperlegiblenext/AtkinsonHyperlegibleNext%5Bwght%5D.ttf",
             "5a455d1cfa099b601ab70751bb9673e8fe1854dc4500c80e1a220d0d75e31745"),
    "mono": ("ofl/atkinsonhyperlegiblemono/AtkinsonHyperlegibleMono%5Bwght%5D.ttf",
             "5ce8b1698d1ded7dff2178c1a3ad159470085a58ea239e8b2cb88f4fb4a6f646"),
}
INTER_ZIP = ("https://github.com/rsms/inter/releases/download/v4.1/Inter-4.1.zip",
             "9883fdd4a49d4fb66bd8177ba6625ef9a64aa45899767dde3d36aa425756b11e")
INTER_TTF = ("extras/ttf/Inter-Bold.ttf", "288316099b1e0a47a4716d159098005eef7c0066921f34e3200393dbdb01947f")
INTER_LICENCE = "LICENSE.txt"
KERN_MAX = 10920        # 1 format 0 subtable: its 16-bit length holds 10920 pairs of 6 bytes


def sha256(path):
    with open(path, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


def download(url, local, sha):
    if not os.path.exists(local):
        os.makedirs(os.path.dirname(local), exist_ok=True)
        with urllib.request.urlopen(url, timeout=300) as r:
            data = r.read()
        if hashlib.sha256(data).hexdigest() != sha:
            sys.exit(f"{url}: sha256 does not match; not using it")
        with open(local + ".part", "wb") as f:
            f.write(data)
        os.replace(local + ".part", local)
    if sha256(local) != sha:
        sys.exit(f"{local}: sha256 does not match; delete it and run again")
    return local


def source(key):
    path, sha = SRC[key]
    local = os.path.join(CACHE, path.rsplit("/", 1)[1].replace("%5B", "[").replace("%5D", "]"))
    return download(f"https://raw.githubusercontent.com/google/fonts/{COMMIT}/{path}", local, sha)


def inter():
    """Inter Bold and its licence, read from the release zip without unpacking anything else."""
    url, sha = INTER_ZIP
    z = download(url, os.path.join(CACHE, url.rsplit("/", 1)[1]), sha)
    ttf = os.path.join(CACHE, "Inter-Bold-4.1.ttf")
    lic = os.path.join(CACHE, "Inter-4.1-LICENSE.txt")
    with zipfile.ZipFile(z) as zf:
        for member, local in ((INTER_TTF[0], ttf), (INTER_LICENCE, lic)):
            with zf.open(member) as src, open(local, "wb") as dst:
                dst.write(src.read())
    if sha256(ttf) != INTER_TTF[1]:
        sys.exit(f"{INTER_TTF[0]} in {z}: sha256 does not match")
    return ttf, lic


# The code points of the site's subsets, read from site.css, so the PDF covers what the site covers.
CSS = open(os.path.join(REPO, "web", "static", "css", "site.css")).read()


def css_range(woff2):
    m = re.search(r"url\(\"?[^\")]*/" + re.escape(woff2) + r"\"?\)[^}]*?unicode-range:\s*([^;]+);", CSS, re.S)
    if not m:
        sys.exit(f"site.css has no unicode-range for {woff2}")
    out = set()
    for part in m.group(1).split(","):
        a, _, b = part.strip()[2:].partition("-")
        out.update(range(int(a, 16), int(b or a, 16) + 1))
    return out


LATIN = css_range("saira-latin-standard-normal.woff2")
LATIN_EXT = css_range("saira-latin-ext-standard-normal.woff2")


def ranges(spec):
    out = set()
    for part in spec.split():
        a, _, b = part.partition("-")
        out.update(range(int(a, 16), int(b or a, 16) + 1))
    return out


# The code points of the Inter cut that the site shipped before (internal/cert/fonts/inter/
# Inter-Bold.ttf, "Inter Bold 4.1, cut to Latin, Greek and Cyrillic"). The release has more (IPA,
# polytonic Greek); keeping the old cut means every name that printed before still prints, and the
# subset stays smaller than the file it replaces.
INTER_CUT = ranges("""
    0000 0020-007E 00A0-00AC 00AE-0148 014A-01C3 01C5-024F 0259 02B0-0304 0306-030A 030C 030F 0313
    0315 031B 0323 0326-0328 032C 0337-0338 0342-0343 0346-036F 0374-0376 037A-037F 0384-038A 038C
    038E-03A1 03A3-03D7 03DC-03DD 03F0-03F6 03F9-03FA 03FC-0479 0480-049D 04A0-04FF 052F 1E00-1E9B
    1E9D-1EFF 2000-200B 2010-2027 202F-2055 2057 205F 20A0-20AF 20B1-20B5 20B8-20BA 20BC-20C0
    2100-2101 2103 2105-2106 2109 2113 2116-2117 211E-2122 2126 212A-212B 212E 2132 213B 214D""")
# Names that Saira cannot set fall back to Inter Bold: Latin, Greek and Cyrillic, with combining marks.
NAMES_FALLBACK = (LATIN | LATIN_EXT | set(range(0x0300, 0x0370)) | set(range(0x0370, 0x0400)) | set(range(0x1F00, 0x2000))
                  | set(range(0x0400, 0x0530)) | set(range(0x1C80, 0x1C90)) | set(range(0x2DE0, 0x2E00))
                  | set(range(0xA640, 0xA6A0)) | set(range(0x1E00, 0x1F00))) & INTER_CUT


def name_chars(cps):
    """Letters, plus what sits inside a name: space, hyphen, apostrophes, full stop."""
    return {c for c in cps if unicodedata.category(chr(c))[0] == "L"} | {0x20, 0x2D, 0x27, 0x2019, 0x2E}


# Kerning sets: only the pairs a certificate can meet. Sentences, labels and titles are English;
# a name is set in Saira Name, or in Inter Names when it holds a rune Saira lacks (Cyrillic, Greek).
TEXT_CHARS = set(range(0x20, 0x7F)) | {0xB7, 0x2018, 0x2019, 0x201C, 0x201D, 0x2013, 0x2014}
KIND_CHARS = set(map(ord, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 ·-/:.,"))
FALLBACK_KERN = name_chars(set(range(0x0400, 0x0460)) | set(range(0x0370, 0x0400)))


def kern_pairs(font, chars, minimum):
    """Flattens the GPOS 'kern' feature into {(left, right): value} for the glyphs of chars.
    Follows the OpenType order: lookups add up; inside 1 lookup the first subtable that applies wins
    (format 1 passes a pair it does not list to the next subtable, format 2 decides every pair)."""
    cmap = font.getBestCmap()
    glyphs = sorted({cmap[c] for c in chars if c in cmap})
    gpos = font["GPOS"].table
    idx = sorted({i for fr in gpos.FeatureList.FeatureRecord if fr.FeatureTag == "kern" for i in fr.Feature.LookupListIndex})
    total = {}
    for li in idx:
        lookup = gpos.LookupList.Lookup[li]
        subs = [s.ExtSubTable if lookup.LookupType == 9 else s for s in lookup.SubTable]
        decided = {}
        for st in subs:
            if getattr(st, "LookupType", 2) != 2:
                continue
            cov = set(st.Coverage.glyphs)
            if st.Format == 1:
                for i, left in enumerate(st.Coverage.glyphs):
                    for pvr in st.PairSet[i].PairValueRecord:
                        key = (left, pvr.SecondGlyph)
                        if key not in decided:
                            decided[key] = (getattr(pvr.Value1, "XAdvance", 0) or 0) if pvr.Value1 else 0
            else:
                c1, c2 = st.ClassDef1.classDefs, st.ClassDef2.classDefs
                for left in glyphs:
                    if left not in cov:
                        continue
                    row = st.Class1Record[c1.get(left, 0)].Class2Record
                    for right in glyphs:
                        key = (left, right)
                        if key in decided:
                            continue
                        k2 = c2.get(right, 0)
                        if k2 < len(row):
                            v = row[k2].Value1
                            decided[key] = (getattr(v, "XAdvance", 0) or 0) if v else 0
        for key, v in decided.items():
            if v:
                total[key] = total.get(key, 0) + v
    keep = set(glyphs)
    pairs = {k: v for k, v in total.items() if k[0] in keep and k[1] in keep and abs(v) >= minimum}
    if len(pairs) > KERN_MAX:
        pairs = dict(sorted(pairs.items(), key=lambda kv: -abs(kv[1]))[:KERN_MAX])
    return pairs


def add_kern(font, pairs):
    st = KernTable_format_0()
    st.version, st.format, st.coverage, st.apple, st.tupleIndex = 0, 0, 1, False, None
    st.kernTable = pairs
    t = newTable("kern")
    t.version, t.kernTables = 0, [st]
    font["kern"] = t


def slash_zero(font):
    """Draws the slash of the site's instrument numeral into the 0: the CSS .gpw-z angle (61 degrees)
    and length (0.58 em), as heavy as the 0's own vertical stroke, centred on the 0."""
    glyf = font["glyf"]
    name = font.getBestCmap()[0x30]
    g = glyf[name]
    coords, ends, _ = g.getCoordinates(glyf)
    contours, start = [], 0
    for e in ends:
        contours.append(list(coords[start:e + 1]))
        start = e + 1

    def area(c):
        return sum(x0 * y1 - x1 * y0 for (x0, y0), (x1, y1) in zip(c, c[1:] + c[:1])) / 2

    def bbox(c):
        xs, ys = [p[0] for p in c], [p[1] for p in c]
        return min(xs), min(ys), max(xs), max(ys)
    outer = max(contours, key=lambda c: abs(area(c)))
    inner = [c for c in contours if c is not outer]
    ox0, oy0, ox1, oy1 = bbox(outer)
    stem = (bbox(inner[0])[0] - ox0) if inner else 0.08 * font["head"].unitsPerEm
    upm = font["head"].unitsPerEm
    half = 0.29 * upm
    dx, dy = math.cos(math.radians(61)), math.sin(math.radians(61))
    nx, ny = -dy, dx                                  # the normal, for the slash's thickness
    cx, cy = (ox0 + ox1) / 2, (oy0 + oy1) / 2
    t = 0.92 * stem / 2
    pts = [(cx - dx * half - nx * t, cy - dy * half - ny * t), (cx + dx * half - nx * t, cy + dy * half - ny * t),
           (cx + dx * half + nx * t, cy + dy * half + ny * t), (cx - dx * half + nx * t, cy - dy * half + ny * t)]
    if (area(pts) > 0) != (area(outer) > 0):          # same winding as the outer contour: the union fills
        pts.reverse()
    pen = TTGlyphPen(font.getGlyphSet())
    font.getGlyphSet()[name].draw(pen)
    pen.moveTo(tuple(map(round, pts[0])))
    for p in pts[1:]:
        pen.lineTo(tuple(map(round, p)))
    pen.closePath()
    ng = pen.glyph()
    ng.flags[0] |= 0x40                               # OVERLAP_SIMPLE: the slash crosses the ring
    glyf[name] = ng
    ng.recalcBounds(glyf)
    return stem


def rename(font, family, style="Regular"):
    name = font["name"]
    keep = {0, 7, 8, 9, 11, 12, 13, 14}               # copyright, designer, licence and their URLs stay
    name.names = [n for n in name.names if n.nameID in keep]
    full = f"{family} {style}" if style != "Regular" else family
    for nid, s in ((1, family), (2, style), (3, f"{family.replace(' ', '')}-{style};gpw-pdf"), (4, full),
                   (5, "Version 1.0; static instance made by go-practice-web scripts/pdf-fonts.py"),
                   (6, f"{family.replace(' ', '')}-{style}")):
        name.setName(s, nid, 3, 1, 0x409)


def build(out, src, axes, cps, kern_chars, kern_min, family, style="Regular", slashed=False):
    font = TTFont(src)
    if axes:
        font = instantiateVariableFont(font, axes, overlap=OverlapMode.KEEP_AND_SET_FLAGS, static=True)
    pairs = kern_pairs(font, kern_chars & cps, kern_min) if kern_chars and "GPOS" in font else {}
    if pairs:
        add_kern(font, pairs)
    stem = slash_zero(font) if slashed else None
    opt = Options()
    opt.layout_features = []
    opt.drop_tables += ["GSUB", "GPOS", "GDEF", "STAT", "MVAR", "HVAR", "VVAR", "avar", "fvar", "DSIG", "meta"]
    opt.name_IDs = ["*"]
    opt.name_languages = ["*"]
    opt.notdef_outline = True
    opt.glyph_names = False
    opt.hinting = True
    opt.legacy_kern = True                            # keep the kern table written above
    sub = Subsetter(opt)
    sub.populate(unicodes=sorted(cps))
    sub.subset(font)
    rename(font, family, style)
    # Same input, same bytes: the fonts are committed, so the save must not stamp today's date.
    font.recalcTimestamp = False
    font["head"].modified = font["head"].created
    font.flavor = None
    path = os.path.join(OUT, out)
    font.save(path)
    return path, len(font.getBestCmap()), len(font.getGlyphOrder()), len(pairs), stem


# Kerning thresholds, in units of a 1000-unit em: the name and the kind line are display sizes
# (26 to 50 pt), where 1.5 % of the em is visible; text is 8.5 to 15 pt, where under 1 % is not.
FONTS = [
    # file, source, axes, code points, kerning set, kerning minimum, family, style, slashed zero
    ("SairaName.ttf", "saira", {"wdth": 66, "wght": 760}, LATIN | LATIN_EXT, name_chars(LATIN | LATIN_EXT) | KIND_CHARS, 15,
     "Saira Name", "Regular", False),
    ("SairaKind.ttf", "saira", {"wdth": 125, "wght": 300}, LATIN, KIND_CHARS, 5, "Saira Kind", "Regular", True),
    ("AtkinsonNext-Regular.ttf", "next", {"wght": 400}, LATIN | LATIN_EXT, TEXT_CHARS, 10,
     "Atkinson Hyperlegible Next PDF", "Regular", False),
    ("AtkinsonNext-Bold.ttf", "next", {"wght": 700}, LATIN | LATIN_EXT, TEXT_CHARS, 10,
     "Atkinson Hyperlegible Next PDF", "Bold", False),
    ("AtkinsonMono-Regular.ttf", "mono", {"wght": 400}, LATIN, set(), 0, "Atkinson Hyperlegible Mono PDF", "Regular", False),
    ("InterBold-Names.ttf", "inter", None, NAMES_FALLBACK, FALLBACK_KERN, 10, "Inter Names", "Bold", False),
]

# The site's licence files are the npm packages' own, the same text as google/fonts ships.
LICENCES = {"saira": "OFL-Saira.txt", "next": "OFL-AtkinsonHyperlegibleNext.txt", "mono": "OFL-AtkinsonHyperlegibleMono.txt"}


def covers(path, text):
    cmap = TTFont(path).getBestCmap()
    return [ch for ch in text if ch != " " and ord(ch) not in cmap]


def main():
    inter_ttf, inter_lic = inter()
    os.makedirs(OUT, exist_ok=True)
    rows = []
    for out, key, axes, cps, kc, kmin, fam, style, slashed in FONTS:
        src = inter_ttf if key == "inter" else source(key)
        path, ncmap, nglyph, npairs, stem = build(out, src, axes, cps, kc, kmin, fam, style, slashed)
        rows.append((out, key, axes, os.path.getsize(path), ncmap, nglyph, npairs, stem))
    for lic in LICENCES.values():
        with open(os.path.join(SITE_FONTS, lic)) as f, open(os.path.join(OUT, lic), "w") as g:
            g.write(f.read())
    with open(inter_lic) as f, open(os.path.join(OUT, "OFL-Inter.txt"), "w") as g:
        g.write(f.read())

    lines = ["Fonts of the certificate PDF. Made by scripts/pdf-fonts.py; do not edit by hand.", "",
             f"Sources: google/fonts at commit {COMMIT} (raw.githubusercontent.com), checked by sha256:"]
    for p, sha in SRC.values():
        lines.append(f"  {p.replace('%5B', '[').replace('%5D', ']')}  sha256 {sha}")
    lines.append(f"  {INTER_TTF[0]} of {INTER_ZIP[0]}  sha256 {INTER_TTF[1]} (Version 4.001)")
    lines += ["", "file                        source  instance                 bytes    KB  code points  glyphs  kern pairs"]
    total = 0
    for out, key, axes, size, ncmap, nglyph, npairs, stem in rows:
        inst = ", ".join(f"{k} {v}" for k, v in axes.items()) if axes else "static, subset"
        total += size
        lines.append(f"{out:27} {key:7} {inst:24} {size:6} {size / 1024:5.1f}  {ncmap:11}  {nglyph:6}  {npairs:10}"
                     + (f"  (slashed 0, stroke {stem:.0f} units)" if stem else ""))
    lines.append(f"{'total':60} {total:6} {total / 1024:5.1f}")
    lines += ["", "The name rule: Saira Name when every rune of the name is in its cmap, else Inter Names for the whole name."]
    for name in ("Łukasz Dvořák-Wiśniewska", "Анна", "Zoë Ngũgĩ", "Ελένη Παπαδοπούλου"):
        miss = covers(os.path.join(OUT, "SairaName.ttf"), name)
        if not miss:
            lines.append(f"  {name}: Saira Name")
        else:
            m2 = covers(os.path.join(OUT, "InterBold-Names.ttf"), name)
            who = "Inter Names" if not m2 else "rejected (no font has " + "".join(m2) + ")"
            lines.append(f"  {name}: {who} (Saira Name lacks {''.join(sorted(set(miss)))})")
    text = "\n".join(lines) + "\n"
    with open(os.path.join(OUT, "SOURCES.txt"), "w") as f:
        f.write(text)
    print(text, end="")


if __name__ == "__main__":
    main()
