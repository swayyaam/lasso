import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

/**
 * A Content-Security-Policy for the built app.
 *
 * CLAUDE.md's network rule — the webview talks to nothing but Lasso itself;
 * thumbnails come through the backend at /thumbs/ — was a convention. This
 * makes the webview enforce it: scripts, styles, images and connections all
 * have to come from the app's own origin. Even if something did manage to put
 * script in the page, it could not load more from anywhere else or send what
 * it found out.
 *
 * Verified against what Wails adds to the page: two same-origin scripts,
 * /wails/runtime.js and /wails/ipc.js, and no inline script or eval. Bindings
 * go through WebKit's message handler, which a policy does not govern.
 *
 * Build only. Vite's development server hot-reloads over a websocket that
 * `connect-src 'self'` would refuse.
 */
function contentSecurityPolicy(): Plugin {
  const policy = [
    "default-src 'self'",
    "script-src 'self'",
    // React and Radix position things with style attributes; that is the only
    // inline anything the app uses.
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data:",
    "font-src 'self' data:",
    "connect-src 'self'",
    "object-src 'none'",
    "base-uri 'none'",
    "form-action 'none'",
  ].join("; ");
  return {
    name: "lasso-content-security-policy",
    apply: "build",
    transformIndexHtml(html) {
      return html.replace(
        "<head>",
        `<head>\n    <meta http-equiv="Content-Security-Policy" content="${policy}" />`,
      );
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), contentSecurityPolicy()],
  // Wails serves the built assets from a custom scheme, so every reference
  // must be relative rather than rooted at /.
  base: "./",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2022",
  },
});
