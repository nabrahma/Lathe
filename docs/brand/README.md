# The mark

A workpiece seen end on, its face broken where the tool has cut, and the tool
sitting in the cut it made. Safety yellow marks the tool because the tool is
the part that does the work, which is the same rule the interface follows.

## Files

| File | Use |
| --- | --- |
| `logo.svg` | The mark alone, on its black tile. Favicons, avatars, anywhere square. |
| `logotype.svg` | Mark plus wordmark. The README header, and anywhere with room for a line. |
| `../../build/appicon.png` | 1024px application icon, generated. Transparent. |
| `../../build/windows/icon.ico` | Windows icon, seven sizes, generated. Transparent. |

The tile is part of the lockup rather than a background, so the SVGs read the
same on a light page and a dark one and need no second variant.

The application icon is the exception: it drops the tile and the corner ticks
and sits on transparency, because it appears in a task bar or a dock beside
other transparent icons and a black square there looks like a mistake. The
generator centres the mark on itself once the tile is gone.

One consequence worth knowing: the workpiece is near white, so on a light task
bar it has little contrast. Lathe's own interface is dark and the icon was
drawn for a dark shell. If that becomes a problem, the fix is to put the tile
back for the icon rather than to recolour the mark.

## Three definitions, one geometry

The same shape is written down three times, in the same 64-unit space:

- `logo.svg` and `logotype.svg`, as SVG
- `frontend/src/components/Logo.tsx`, for the title bar, taking its colour from
  the surrounding text
- `scripts/appicon`, which rasterises it for the application icon and the
  Windows `.ico`

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
| Ticks | 3 in from each corner, 5 long, 1.5 wide (lockup only) |
| Ground | `#080808` |
| Metal | `#ededed` |
| Tool | `#ffe500` |
| Ticks | `#3d3d3d` |
