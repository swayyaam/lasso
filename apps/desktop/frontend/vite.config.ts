import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // Wails serves the built assets from a custom scheme, so every reference
  // must be relative rather than rooted at /.
  base: "./",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2022",
  },
});
