import { mkdir, readdir, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { parse, parseFragment, serialize } from "parse5";
import { createServer } from "vite";

const frontend = fileURLToPath(new URL("../", import.meta.url));
const source = fileURLToPath(new URL("../../static", import.meta.url));
const output = join(source, "workbench", "pages");
const stylesheet = "/admin/static/workbench/index.css";
const types = new Set(["thead", "tbody", "th", "tr", "td"]);
const attr = (node, name) => node.attrs?.find((item) => item.name === name)?.value;
const pages = (await readdir(source)).filter((name) => name.endsWith(".html")).sort();
if (!pages.length) throw new Error("No admin HTML templates found in " + source);

function each(node, visit, inTable = false) {
  visit(node, inTable);
  node.childNodes?.forEach((child) => each(child, visit, inTable || node.tagName === "table"));
  if (node.content) each(node.content, visit, inTable);
}

function componentFor(node, inTable) {
  if (!node.attrs || node.namespaceURI !== "http://www.w3.org/1999/xhtml") return;
  const classes = new Set((attr(node, "class") || "").split(/\s+/));
  const tag = node.tagName;
  if (classes.has("btn") && ["button", "a"].includes(tag)) {
    const variant = classes.has("btn-danger") ? "destructive"
      : classes.has("btn-link") ? "link"
      : [...classes].some((value) => value.startsWith("btn-outline-")) ? "outline"
      : classes.has("btn-secondary") ? "secondary"
      : classes.has("btn-primary") || classes.has("btn-success") ? "default"
      : "ghost";
    return ["button", { variant, size: classes.has("btn-icon") ? "icon" : classes.has("btn-sm") ? "sm" : "default" }];
  }
  if (classes.has("badge")) return ["badge", { variant: "outline" }];
  if (classes.has("alert")) return ["alert", { variant: classes.has("alert-danger") ? "destructive" : "default" }];
  if (classes.has("card") || classes.has("panel") || classes.has("login-box")) return ["card", {}];
  if (classes.has("card-header")) return ["card-header", {}];
  if (classes.has("card-body")) return ["card-content", {}];
  if (tag === "input" && classes.has("form-control")) return ["input", {}];
  if (tag === "textarea" && classes.has("form-control")) return ["textarea", {}];
  if (tag === "table" && (classes.has("table") || classes.has("data"))) return ["table", {}];
  if (inTable && types.has(tag)) return [tag, {}];
}

function recipeFrom(markup, kind) {
  // Rows and cells require a table parsing context. The source table keeps its original children.
  if (["thead", "tbody", "th", "tr", "td"].includes(kind)) {
    if (kind === "th" || kind === "td") markup = "<table><tbody><tr>" + markup + "</tr></tbody></table>";
    else markup = "<table>" + markup + "</table>";
  }
  let recipe;
  each(parseFragment(markup), (node) => {
    const slot = attr(node, "data-slot");
    if (slot && slot !== "table-container") recipe ||= node.attrs;
  });
  return recipe || [];
}

function applyRecipe(node, recipe) {
  for (const item of recipe) {
    if (item.name !== "class" && item.name !== "data-slot") continue;
    const existing = node.attrs.find((attribute) => attribute.name === item.name);
    if (!existing) node.attrs.push({ ...item });
    else if (item.name === "class") existing.value += " " + item.value;
  }
}

const vite = await createServer({ root: frontend, server: { middlewareMode: true, hmr: false, ws: false, watch: null }, appType: "custom" });
try {
  const { componentMarkup, sidebarPartsMarkup } = await vite.ssrLoadModule("/src/static-components.jsx");
  const sidebarParts = sidebarPartsMarkup();
  const recipes = new Map();
  await mkdir(output, { recursive: true });
  for (const name of pages) {
    const html = await readFile(join(source, name), "utf8");
    const doc = parse(html);
    each(doc, (node, inTable) => {
      const config = componentFor(node, inTable);
      if (!config) return;
      const key = JSON.stringify(config);
      if (!recipes.has(key)) recipes.set(key, recipeFrom(componentMarkup(...config), config[0]));
      applyRecipe(node, recipes.get(key));
    });
    let compiled = serialize(doc);
    if (compiled.includes('src="/admin/static/shell.js"')) {
      compiled = compiled.replace("</head>", sidebarParts + "</head>");
    }
    if (!compiled.includes(stylesheet)) {
      compiled = compiled.replace("</head>", '<link rel="stylesheet" href="' + stylesheet + '"></head>');
    }
    await writeFile(join(output, name), compiled);
  }
  process.stdout.write("Rendered " + pages.length + " admin templates with shadcn/ui.\n");
} finally {
  await vite.close();
}
