import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    // The bundle is embedded in the binary, so a small one is a smaller
    // installer. Chunk splitting would only add requests inside a webview.
    target: "es2022",
    cssCodeSplit: false,
    reportCompressedSize: false,
  },
});
