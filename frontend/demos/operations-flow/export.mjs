// Build a file://-friendly, self-contained presentation without dependencies.
import { readFile, writeFile } from 'node:fs/promises';
const root = new URL('./', import.meta.url);
const html = await readFile(new URL('index.html',root),'utf8');
const css = await readFile(new URL('style.css',root),'utf8');
const modules = await Promise.all(['audience.mjs','model.mjs','experiment-ui.mjs','campaign-activities.mjs','campaign-ui.mjs','audience-ui.mjs','app.mjs'].map(name=>readFile(new URL(name,root),'utf8')));
const script = modules.map(source=>source.replace(/^import .* from ['"].*['"];\n/gm,'').replace(/^export /gm,'')).join('\n');
if (/<\/script/i.test(script)||/<\/style/i.test(css)) throw new Error('Unsafe inline closing tag');
const output = process.argv[2];
if (!output) throw new Error('Pass the output HTML path');
const result = html.replace('<link rel="stylesheet" href="./style.css">',`<style>\n${css}\n</style>`).replace('<script type="module" src="./app.mjs"></script>','').replace('</body>',`<script>\n(() => {\n${script}\n})();\n</script>\n</body>`);
await writeFile(output,result);
console.log(output);
