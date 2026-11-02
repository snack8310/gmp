// Build a file://-friendly, self-contained presentation without dependencies.
import { readFile, writeFile } from 'node:fs/promises';
const root = new URL('./', import.meta.url);
const html = await readFile(new URL('index.html',root),'utf8');
const styles = await Promise.all(['style.css','activity-navigation.css'].map(name=>readFile(new URL(name,root),'utf8')));
const css = styles.join('\n');
const modules = await Promise.all(['audience.mjs','experiment-ui.mjs','activity-routing.mjs','activity-navigation.mjs','campaign-activities.mjs','campaign-ui.mjs','campaign-api-client.mjs','campaign-controller.mjs','audience-ui.mjs','app.mjs'].map(name=>readFile(new URL(name,root),'utf8')));
const script = modules.map(source=>source.replace(/^import .* from ['"].*['"];\n/gm,'').replace(/^export /gm,'')).join('\n');
if (/<\/script/i.test(script)||/<\/style/i.test(css)) throw new Error('Unsafe inline closing tag');
const output = process.argv[2];
if (!output) throw new Error('Pass the output HTML path');
const result = html.replace('<link rel="stylesheet" href="./style.css">',`<style>\n${css}\n</style>`).replace('<link rel="stylesheet" href="./activity-navigation.css">','').replace('<script type="module" src="./app.mjs"></script>','').replace('<section id="activity-landing" class="activity-landing">','<section id="activity-landing" class="activity-landing"><p class="storage-warning">活动列表与工作区需要本机管理 API；离线文件不会用静态活动数据替代 API。</p>').replace('</body>',`<script>\n(() => {\n${script}\n})();\n</script>\n</body>`);
await writeFile(output,result);
console.log(output);
