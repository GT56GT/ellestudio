import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build lands in internal/admin/dist, which the Go server embeds
// (go build -tags embedui). `npm run dev` proxies the API to a local panel.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../internal/admin/dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    proxy: { "/api": "http://localhost:8080" },
  },
});
