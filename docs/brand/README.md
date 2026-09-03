# The mark

A workpiece seen end on, its face broken where the tool has cut, and the tool
sitting in the cut it made. Safety yellow marks the tool because the tool is
the part that does the work, which is the same rule the interface follows.

## Files

| File | Use |
| --- | --- |
| `logo.svg` | The mark alone, on its black tile. Favicons, avatars, anywhere square. |
| `logotype.svg` | Mark plus wordmark. The README header, and anywhere with room for a line. |
| `../../build/appicon.png` | 1024px application icon, generated. |

The tile is part of the mark rather than a background, so it reads the same on
a light page and a dark one and needs no second variant.

## Three definitions, one geometry

The same shape is written down three times, in the same 64-unit space:

- `logo.svg` and `logotype.svg`, as SVG
- `frontend/src/components/Logo.tsx`, for the title bar, taking its colour from
  the surrounding text
- `scripts/appicon`, which rasterises it

Changing one means changing all of them. The Go renderer exists because the
project has no SVG rasteriser in its toolchain and adding one for a single
image was not worth it.

Regenerate the application icon with:

```sh
make appicon
```

## Numbers

| | |
| --- | --- |
| Outer face | r 22, stroke 4, broken over 26 degrees centred on the horizontal |
| Centre | r 5.5, filled |
| Tool | tip at x 47, shoulder at x 56, half height 4.5 |
| Ticks | 3 in from each corner, 5 long, 1.5 wide |
| Ground | `#080808` |
| Metal | `#ededed` |
| Tool | `#ffe500` |
| Ticks | `#3d3d3d` |
