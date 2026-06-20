import { copyFile, mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import esbuild from "esbuild";

const root = dirname(fileURLToPath(import.meta.url));
const packageRoot = join(root, "..");
const dist = join(packageRoot, "dist");

await mkdir(dist, { recursive: true });

await esbuild.build({
  entryPoints: [join(packageRoot, "src/main.ts")],
  outfile: join(dist, "main.js"),
  bundle: true,
  target: "es2017",
  format: "iife"
});

const uiBuild = await esbuild.build({
  entryPoints: [join(packageRoot, "src/ui.ts")],
  bundle: true,
  target: "es2017",
  format: "iife",
  write: false
});

const uiHtml = await readFile(join(packageRoot, "src/ui.html"), "utf8");
const uiJs = uiBuild.outputFiles[0].text;
await writeFile(
  join(dist, "ui.html"),
  uiHtml.replace('<script src="ui.js"></script>', `<script>\n${uiJs}\n</script>`)
);
await copyFile(join(packageRoot, "src/manifest.json"), join(dist, "manifest.json"));
