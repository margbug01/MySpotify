"""Turns the readme scene's PNGs into the README's WebP screenshots: rounded
corners, a soft shadow, and a hero of the home page split light/dark along a
diagonal. usage: readme_shots.py <shots dir> <out dir>"""
import os, sys
from PIL import Image, ImageDraw, ImageFilter

src, out = sys.argv[1], sys.argv[2]
os.makedirs(out, exist_ok=True)
W = 1400          # width of the window in the image
R = 22            # its corner radius
PAD = 40          # room for the shadow


def framed(im):
    im = im.convert('RGB')
    h = round(im.height * W / im.width)
    im = im.resize((W, h), Image.LANCZOS)
    mask = Image.new('L', (W, h), 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, W - 1, h - 1), R, fill=255)
    canvas = Image.new('RGBA', (W + 2 * PAD, h + 2 * PAD), (0, 0, 0, 0))
    shadow = Image.new('L', canvas.size, 0)
    ImageDraw.Draw(shadow).rounded_rectangle((PAD, PAD + 10, PAD + W, PAD + h + 10), R, fill=95)
    shadow = shadow.filter(ImageFilter.GaussianBlur(18))
    canvas.paste((0, 0, 0, 255), (0, 0), shadow)
    win = im.convert('RGBA')
    win.putalpha(mask)
    canvas.alpha_composite(win, (PAD, PAD))
    # A hairline edge, as a window has.
    edge = Image.new('RGBA', (W, h), (0, 0, 0, 0))
    ImageDraw.Draw(edge).rounded_rectangle((0, 0, W - 1, h - 1), R, outline=(0, 0, 0, 40), width=1)
    canvas.alpha_composite(edge, (PAD, PAD))
    return canvas


def save(img, name):
    path = os.path.join(out, name + '.webp')
    img.save(path, 'WEBP', quality=82, method=6)
    return os.path.getsize(path)


def load(n):
    return Image.open(os.path.join(src, n + '.png'))


# The hero: light above the diagonal from top right to bottom left, dark
# below it.
light, dark = load('r1-home').convert('RGB'), load('r11-home-dark').convert('RGB')
dark = dark.resize(light.size)
cut = Image.new('L', light.size, 0)
w, h = light.size
ImageDraw.Draw(cut).polygon([(w * 0.62, 0), (w, 0), (w, h), (w * 0.38, h)], fill=255)
hero = light.copy()
hero.paste(dark, (0, 0), cut)
total = save(framed(hero), 'hero')

for name, shot in [('playing', 'r2-playing'), ('playing-dark', 'r12-playing-dark'), ('album', 'r3-album'),
                   ('artist', 'r4-artist'), ('search', 'r5-search'), ('browse', 'r6-browse'),
                   ('podcast', 'r7-podcast'), ('track', 'r8-track'), ('category', 'r9-category'), ('profile', 'r10-profile')]:
    total += save(framed(load(shot)), name)
print('total KB', total // 1024, sorted(os.listdir(out)))
