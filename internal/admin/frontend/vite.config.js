import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

const localPath = (path) => fileURLToPath(new URL(path, import.meta.url));
const license = readFileSync(localPath("./SHADCN-LICENSE.md"), "utf8").trimEnd();

export default defineConfig({
  plugins: [tailwindcss()],
  resolve: { alias: { "@": localPath("./src") } },
  esbuild: { jsx: "automatic", legalComments: "inline" },
  build: {
    target: "es2022",
    outDir: localPath("../static/workbench"),
    emptyOutDir: true,
    rollupOptions: {
      input: localPath("./src/workbench.jsx"),
      output: {
        entryFileNames: "index.js",
        assetFileNames: "index[extname]",
        inlineDynamicImports: true,
        banner: "/*!\n" + license + "\n*/",
      },
    },
  },
});
