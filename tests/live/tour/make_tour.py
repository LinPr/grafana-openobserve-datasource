#!/usr/bin/env python3
"""Assemble the recorded frames into the README tour.

The frames come from record.mjs (real screenshots of a live Grafana). Two outputs
are produced from them:

  doc/screenshot/tour.webp  high-quality animation, small
  doc/screenshot/tour.gif   downscaled animation for renderers that ignore WebP

GIF has no inter-frame differencing in Pillow, so its size is roughly
frames x pixels. The ladder below keeps the width legible (>=700px) and instead
drops frames until the file fits the budget.

Usage:
    python3 tests/live/tour/make_tour.py
"""
import glob
import hashlib
import os
import sys

from PIL import Image

SRC_DIR = os.path.join(os.path.dirname(__file__), 'frames')
GIF_OUT = 'doc/screenshot/tour.gif'
WEBP_OUT = 'doc/screenshot/tour.webp'

CAPTURE_MS = 100      # record.mjs cadence
GIF_BUDGET_MB = 6.5


def distinct_groups():
    """Collapse runs of identical frames (held views) into single long frames."""
    files = sorted(glob.glob(os.path.join(SRC_DIR, 'f*.png')))
    if not files:
        sys.exit(f'no frames in {SRC_DIR}; run record.mjs first')

    groups = []
    previous = None
    for path in files:
        digest = hashlib.md5(open(path, 'rb').read()).hexdigest()
        if digest == previous:
            groups[-1] = (groups[-1][0], groups[-1][1] + 1)
        else:
            groups.append((path, 1))
            previous = digest
    print(f'{len(files)} captured frames -> {len(groups)} distinct')
    return groups


def load(groups, width):
    base = Image.open(groups[0][0])
    height = round(base.height * (width / base.width))
    images = [Image.open(p).convert('RGB').resize((width, height), Image.LANCZOS) for p, _ in groups]
    return images, height


def build_gif(groups, width, colors, keep_every, out):
    picks = groups[::keep_every]
    images, height = load(picks, width)
    durations = [count * CAPTURE_MS * keep_every for _, count in picks]

    # One shared palette keeps colours stable across scenes and compresses much
    # better than per-frame palettes.
    sample = Image.new('RGB', (width, height * 6))
    for i, idx in enumerate(range(0, len(images), max(1, len(images) // 6))[:6]):
        sample.paste(images[idx], (0, i * height))
    palette = sample.quantize(colors=colors, method=Image.MEDIANCUT)

    # Full frames + disposal=2 so no residue leaks between scenes.
    frames = [im.quantize(palette=palette) for im in images]
    frames[0].save(out, format='GIF', save_all=True, append_images=frames[1:],
                   duration=durations, loop=0, optimize=True, disposal=2)

    size = os.path.getsize(out) / 1024 / 1024
    print(f'  gif width={width} colors={colors} every={keep_every} '
          f'frames={len(frames)} dur={sum(durations) / 1000:.1f}s -> {size:.2f} MiB')
    return size


def build_webp(groups, width=900, quality=78):
    images, height = load(groups, width)
    durations = [count * CAPTURE_MS for _, count in groups]
    images[0].save(WEBP_OUT, format='WEBP', save_all=True, append_images=images[1:],
                   duration=durations, loop=0, quality=quality, method=5)
    print(f'{WEBP_OUT}: {os.path.getsize(WEBP_OUT) / 1024 / 1024:.2f} MiB '
          f'({width}x{height}, {len(images)} frames, {sum(durations) / 1000:.1f}s)')


def main():
    groups = distinct_groups()
    os.makedirs('doc/screenshot', exist_ok=True)

    # Legibility first: never go below 700px wide just to save bytes.
    ladder = [(760, 128, 2), (720, 96, 3), (700, 96, 4), (700, 64, 4), (700, 64, 5)]
    tmp = GIF_OUT + '.tmp.gif'
    for width, colors, keep_every in ladder:
        if build_gif(groups, width, colors, keep_every, tmp) <= GIF_BUDGET_MB:
            os.replace(tmp, GIF_OUT)
            print(f'accepted -> {GIF_OUT} ({os.path.getsize(GIF_OUT) / 1024 / 1024:.2f} MiB)')
            break
    else:
        os.replace(tmp, GIF_OUT)
        print(f'accepted last candidate -> {GIF_OUT}')

    build_webp(groups)


if __name__ == '__main__':
    main()
