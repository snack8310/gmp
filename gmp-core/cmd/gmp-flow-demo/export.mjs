// Build a file://-friendly, self-contained presentation without dependencies.
import { readFile, writeFile } from 'node:fs/promises';
const root = new URL('./web/', import.meta.url);
const [html, css, model, app] = await Promise.all(['index.html','style.css','model.mjs','app.mjs'].map(name=>readFile(new URL(name,root),'utf8')));
const inlineModel = model.replace(/^export /gm, '');
const inlineApp = app.replace(/^import .* from '\.\/model\.mjs';\n/, '');
if (/<\/script/i.test(inlineModel+inlineApp)||/<\/style/i.test(css)) throw new Error('Unsafe inline closing tag');
const output = process.argv[2];
if (!output) throw new Error('Pass the output HTML path');
const result = html.replace('<link rel="stylesheet" href="./style.css">',`<style>\n${css}\n</style>`).replace('<script type="module" src="./app.mjs"></script>','').replace('</body>',`<script>\n(() => {\n${inlineModel}\n${inlineApp}\n})();\n</script>\n</body>`);
await writeFile(output,result);
console.log(output);
