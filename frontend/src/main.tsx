import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import "./styles/tokens.css";
import "./styles/base.css";
import "./styles/components.css";

/*
 * Browser behaviours that have no place in an application window. Each is a
 * one-line fix and together they remove most of the webbiness in one pass.
 */

// The browser's own right-click menu.
window.addEventListener("contextmenu", (e) => e.preventDefault());

// Ctrl+scroll and Ctrl+plus zooming the whole interface.
window.addEventListener(
  "wheel",
  (e) => {
    if (e.ctrlKey) e.preventDefault();
  },
  { passive: false },
);

window.addEventListener("keydown", (e) => {
  const mod = e.ctrlKey || e.metaKey;
  if (!mod) return;

  // Ctrl+F opening a browser find bar, and zoom shortcuts.
  if (["f", "+", "-", "=", "0", "p"].includes(e.key.toLowerCase())) {
    e.preventDefault();
  }
});

// A drag that ends anywhere other than a drop zone must not navigate the
// window to the dropped file.
window.addEventListener("dragover", (e) => e.preventDefault());
window.addEventListener("drop", (e) => e.preventDefault());

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
