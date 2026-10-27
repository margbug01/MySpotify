"""Draws Vinyl's icons. Run on Windows, with Pillow: python tools/icons.py

The record is drawn here, and every icon made of it:

  resources/icon.svg, icon.png   the app's icon, 1024 px (Linux, macOS's .icns)
  resources/icon-small.svg       the same record for 16-24 px, two grooves bolder
  resources/icon.ico             Windows: icon-small for 16-24 px, icon above
  internal/app/tray.png          the notification area's and the menu bar's,
                                 a template: black and clear, which Windows draws
                                 white on a dark taskbar and macOS tints itself
  AppIcon.icon                   the Dock's, an Icon Composer document whose tile
                                 is light or dark as macOS is (mygo.json macos.icon)
  docs/icon.png                  the README's, 256 px

Edge renders the SVGs. Then rsrc_windows_amd64.syso again, as the README says.
"""
import json
import math
import os
import shutil
import subprocess
import tempfile

from PIL import Image

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
EDGE = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
C = 512
INK, RIM, LIGHT = '#0a0a0a', '#5c5c5c', '#ffffff'


def arc(r, a0, a1, width, color):
    x0, y0 = C + r * math.cos(math.radians(a0)), C + r * math.sin(math.radians(a0))
    x1, y1 = C + r * math.cos(math.radians(a1)), C + r * math.sin(math.radians(a1))
    large = 1 if (a1 - a0) % 360 > 180 else 0
    return (f'<path d="M{x0:.1f} {y0:.1f} A{r} {r} 0 {large} 1 {x1:.1f} {y1:.1f}" fill="none" '
            f'stroke="{color}" stroke-width="{width}" stroke-linecap="round"/>')


def grooves(color, small=False):
    """The light the record catches as it turns: three grooves, each a little
    further round, and the hole; two, bolder, where pixels are few."""
    if small:
        return (arc(330, 150, 270, 96, color) + arc(182, 210, 318, 90, color)
                + f'<circle cx="{C}" cy="{C}" r="64" fill="{color}"/>')
    return (arc(330, 150, 270, 62, color) + arc(242, 205, 305, 54, color) + arc(156, 255, 335, 46, color)
            + f'<circle cx="{C}" cy="{C}" r="44" fill="{color}"/>')


def record(small=False):
    rim = 36 if small else 22  # the edge a dark taskbar or Dock would otherwise swallow
    return (f'<circle cx="{C}" cy="{C}" r="448" fill="{INK}"/>'
            f'<circle cx="{C}" cy="{C}" r="{448 - rim / 2}" fill="none" stroke="{RIM}" stroke-width="{rim}"/>'
            + grooves(LIGHT, small))


def svg(body, defs=''):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">'
            f'<defs>{defs}</defs>{body}</svg>\n')


def write(path, text):
    with open(os.path.join(ROOT, path), 'w', encoding='utf-8', newline='\n') as f:
        f.write(text)


def render(svg_text, work):
    """svg_text as a 1024 px image, on nothing."""
    src, out = os.path.join(work, 'in.svg'), os.path.join(work, 'out.png')
    with open(src, 'w', encoding='utf-8') as f:
        f.write(svg_text)
    subprocess.run([EDGE, '--headless=new', '--disable-gpu', '--hide-scrollbars', '--user-data-dir=' + os.path.join(work, 'edge'),
                    '--default-background-color=00000000', '--window-size=1024,1024', '--screenshot=' + out,
                    'file:///' + src.replace(os.sep, '/')], check=True, capture_output=True)
    with Image.open(out) as im:
        return im.convert('RGBA')


def sized(im, px):
    return im.resize((px, px), Image.LANCZOS)


def main():
    big, small = svg(record()), svg(record(small=True))
    mask = f'<mask id="m"><rect width="1024" height="1024" fill="#fff"/>{grooves("#000", small=True)}</mask>'
    tray = svg(f'<circle cx="{C}" cy="{C}" r="448" fill="#000" mask="url(#m)"/>', mask)
    # The Dock's tile is the whole canvas; the record takes three quarters of it.
    dock = svg(f'<g transform="translate({C} {C}) scale({384 / 448:.5f}) translate(-{C} -{C})">{record()}</g>')
    write('resources/icon.svg', big)
    write('resources/icon-small.svg', small)

    work = tempfile.mkdtemp()
    try:
        big_im, small_im, tray_im, dock_im = (render(s, work) for s in (big, small, tray, dock))
    finally:
        shutil.rmtree(work, ignore_errors=True)

    big_im.save(os.path.join(ROOT, 'resources/icon.png'))
    sized(big_im, 256).save(os.path.join(ROOT, 'docs/icon.png'))
    frames = [sized(small_im, px) for px in (16, 20, 24)] + [sized(big_im, px) for px in (32, 48, 64, 128, 256)]
    frames[-1].save(os.path.join(ROOT, 'resources/icon.ico'), sizes=[f.size for f in frames], append_images=frames[:-1])

    # A template is black where it shows: only its alpha says where.
    t = sized(tray_im, 32)
    black = Image.new('L', t.size, 0)
    Image.merge('RGBA', (black, black, black, t.getchannel('A'))).save(os.path.join(ROOT, 'internal/app/tray.png'))

    icon = os.path.join(ROOT, 'AppIcon.icon')
    os.makedirs(os.path.join(icon, 'Assets'), exist_ok=True)
    dock_im.save(os.path.join(icon, 'Assets', 'record.png'))
    doc = {
        'fill-specializations': [
            {'value': {'solid': 'extended-gray:0.97000,1.00000'}},
            {'appearance': 'dark', 'value': {'solid': 'extended-gray:0.12000,1.00000'}},
        ],
        'groups': [{'layers': [{'image-name': 'record.png', 'name': 'record'}]}],
        'supported-platforms': {'squares': 'shared'},
    }
    with open(os.path.join(icon, 'icon.json'), 'w', encoding='utf-8', newline='\n') as f:
        json.dump(doc, f, indent=2)
        f.write('\n')


if __name__ == '__main__':
    main()
