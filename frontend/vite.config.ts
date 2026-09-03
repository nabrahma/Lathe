import { writeFileSync } from "node:fs";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

/*
 * dist/ is committed as an empty directory so that go:embed resolves in a
 * fresh checkout, before anyone has built the interface. Vite empties the
 * directory on every build, which would delete the marker and silently break
 * the Go build in CI, so it is put back afterwards.
 */
function keepEmbedMarker() {
  return {
    name: "keep-embed-marker",
    closeBundle() {
      writeFileSync(new URL("./dist/.gitkeep", import.meta.url), "");
    },
  };
}

export default defineConfig({
  plugins: [react(), keepEmbedMarker()],
  build: {
    // The bundle is embedded in the binary, so a small one is a smaller
    // installer. Chunk splitting would only add requests inside a webview.
    target: "es2022",
    cssCodeSplit: false,
    reportCompressedSize: false,
  },
});
