#!/usr/bin/env python3
"""Generate GoShareIt tray icons: capture-corner brackets + center dot.
Outputs: tray_darwin.png (black template glyph, 44x44) and
tray_windows.ico (white glyph w/ dark outline, 16/24/32/48)."""
from PIL import Image, ImageDraw
import sys, os

OUT = sys.argv[1]

def glyph(size, color, outline=None):
    # Draw at 8x supersample for clean downscale.
    S = size * 8
    img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    m = S * 0.10          # margin
    arm = S * 0.26        # bracket arm length
    w = max(S * 0.085, 8) # stroke width

    def bracket(cx_edge, cy_edge):
        x = m if cx_edge == 0 else S - m
        y = m if cy_edge == 0 else S - m
        dx = 1 if cx_edge == 0 else -1
        dy = 1 if cy_edge == 0 else -1
        # horizontal arm
        d.rounded_rectangle(
            sorted_box(x, y - (w / 2) * dy_off(dy), x + arm * dx, y + (w / 2) * dy_off(dy), w, dy, True),
            radius=w / 2, fill=color)
        # vertical arm
        d.rounded_rectangle(
            sorted_box(x - (w / 2) * 1, y, x + (w / 2) * 1, y + arm * dy, w, dy, False),
            radius=w / 2, fill=color)

    def dy_off(_):
        return 1

    def sorted_box(x0, y0, x1, y1, w, dsign, horiz):
        xs = sorted((x0, x1)); ys = sorted((y0, y1))
        return [xs[0], ys[0], xs[1], ys[1]]

    # Four corner brackets
    for cx in (0, 1):
        for cy in (0, 1):
            x = m if cx == 0 else S - m
            y = m if cy == 0 else S - m
            dx = 1 if cx == 0 else -1
            dy = 1 if cy == 0 else -1
            # horizontal arm (rounded caps via rounded_rectangle)
            hx0, hx1 = sorted((x, x + arm * dx))
            d.rounded_rectangle([hx0, y - w / 2, hx1, y + w / 2], radius=w / 2, fill=color)
            vy0, vy1 = sorted((y, y + arm * dy))
            d.rounded_rectangle([x - w / 2, vy0, x + w / 2, vy1], radius=w / 2, fill=color)

    # Center dot
    r = S * 0.115
    c = S / 2
    if outline:
        d.ellipse([c - r - w * 0.5, c - r - w * 0.5, c + r + w * 0.5, c + r + w * 0.5], fill=outline)
    d.ellipse([c - r, c - r, c + r, c + r], fill=color)

    return img.resize((size, size), Image.LANCZOS)

# macOS template: pure black + alpha.
glyph(44, (0, 0, 0, 255)).save(os.path.join(OUT, "tray_darwin.png"))

# Windows: white glyph, subtle dark outline on the dot for light taskbars.
sizes = [16, 24, 32, 48]
imgs = [glyph(s, (255, 255, 255, 255), outline=(40, 40, 40, 200)) for s in sizes]
imgs[-1].save(os.path.join(OUT, "tray_windows.ico"),
              sizes=[(s, s) for s in sizes],
              append_images=imgs[:-1])
print("wrote", os.listdir(OUT))
