#!/usr/bin/env python3
"""Generate the tray icon assets.

- internal/icon/tray_darwin.png: 44px monochrome TEMPLATE glyph (capture
  corners + center dot), drawn programmatically. The macOS menu bar renders
  templates black/white adaptively, so the simple glyph reads better there
  than a silhouette of the full logo.
- internal/icon/tray_windows.ico: the product logo (shutter aperture + G,
  build/icons/goshareit_icon.png) as a full-color multi-size ICO
  (16/24/32/48/64) for the Windows tray.

Run from the repo root: python3 scripts/gen-tray-icon.py
"""
from PIL import Image, ImageDraw
import os

OUT = "internal/icon"
LOGO = "build/icons/goshareit_icon.png"


def glyph(size, color):
    """Capture-corner brackets + center dot, drawn at 8x and downsampled."""
    S = size * 8
    img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    m = S * 0.10
    arm = S * 0.26
    w = max(S * 0.085, 8)
    for cx in (0, 1):
        for cy in (0, 1):
            x = m if cx == 0 else S - m
            y = m if cy == 0 else S - m
            dx = 1 if cx == 0 else -1
            dy = 1 if cy == 0 else -1
            hx0, hx1 = sorted((x, x + arm * dx))
            d.rounded_rectangle([hx0, y - w / 2, hx1, y + w / 2], radius=w / 2, fill=color)
            vy0, vy1 = sorted((y, y + arm * dy))
            d.rounded_rectangle([x - w / 2, vy0, x + w / 2, vy1], radius=w / 2, fill=color)
    r = S * 0.115
    c = S / 2
    d.ellipse([c - r, c - r, c + r, c + r], fill=color)
    return img.resize((size, size), Image.LANCZOS)


# macOS template: pure black + alpha.
glyph(44, (0, 0, 0, 255)).save(os.path.join(OUT, "tray_darwin.png"))

# Windows: full-color logo ICO.
logo = Image.open(LOGO).convert("RGBA")
sizes = [16, 24, 32, 48, 64]
imgs = [logo.resize((s, s), Image.LANCZOS) for s in sizes]
imgs[-1].save(os.path.join(OUT, "tray_windows.ico"),
              sizes=[(s, s) for s in sizes], append_images=imgs[:-1])
print("wrote", os.listdir(OUT))
