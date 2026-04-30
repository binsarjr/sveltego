import { glob } from 'tinyglobby';
import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import type { Plugin } from 'vite';

const SITE_URL = 'https://sveltego.dev';
const SIZE_WARN = 500 * 1024;

interface PageMeta {
  file: string;
  slug: string;
  title: string;
  summary: string;
  order: number;
  body: string;
}

function stripFrontmatter(src: string): { meta: Record<string, string>; body: string } {
  if (!src.startsWith('---\n')) {
    return { meta: {}, body: src };
  }
  const end = src.indexOf('\n---\n', 4);
  if (end === -1) {
    return { meta: {}, body: src };
  }
  const fm = src.slice(4, end);
  const body = src.slice(end + 5);
  const meta: Record<string, string> = {};
  for (const line of fm.split('\n')) {
    const i = line.indexOf(':');
    if (i === -1) continue;
    const k = line.slice(0, i).trim();
    let v = line.slice(i + 1).trim();
    if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) {
      v = v.slice(1, -1);
    }
    meta[k] = v;
  }
  return { meta, body };
}

function firstHeading(body: string): string {
  for (const line of body.split('\n')) {
    if (line.startsWith('# ')) return line.slice(2).trim();
  }
  return '';
}

function slugOf(file: string): string {
  if (file === 'index.md') return '';
  return file.replace(/\.md$/, '');
}

export default function llmsTxtPlugin(): Plugin {
  let outDir = '';
  let docsRoot = '';
  return {
    name: 'sveltego:llms-txt',
    apply: 'build',
    configResolved(cfg) {
      outDir = cfg.build.outDir;
      docsRoot = cfg.root;
    },
    async closeBundle() {
      if (!outDir || !docsRoot) return;
      const files = await glob(['**/*.md', '!.vitepress/**', '!node_modules/**'], {
        cwd: docsRoot,
      });
      const pages: PageMeta[] = [];
      for (const file of files) {
        const src = await readFile(path.join(docsRoot, file), 'utf8');
        const { meta, body } = stripFrontmatter(src);
        const title = meta.title || firstHeading(body) || file;
        const summary = meta.summary || '';
        const order = meta.order ? Number(meta.order) : 999;
        pages.push({ file, slug: slugOf(file), title, summary, order, body });
      }
      pages.sort((a, b) => a.order - b.order || a.file.localeCompare(b.file));

      const indexLines: string[] = [
        '# sveltego',
        '',
        '> SvelteKit-shape framework for Go. SSR via codegen, no JS server runtime.',
        '',
        '## Docs',
        '',
      ];
      for (const p of pages) {
        const url = p.slug === '' ? SITE_URL : `${SITE_URL}/${p.slug}`;
        const summary = p.summary ? `: ${p.summary}` : '';
        indexLines.push(`- [${p.title}](${url})${summary}`);
      }
      const index = indexLines.join('\n') + '\n';
      await writeFile(path.join(outDir, 'llms.txt'), index);

      const parts: string[] = [];
      for (const p of pages) {
        parts.push(`# ${p.title}\n\n${p.body.trim()}\n`);
      }
      const full = parts.join('\n---\n\n');
      await writeFile(path.join(outDir, 'llms-full.txt'), full);

      const size = Buffer.byteLength(full, 'utf8');
      if (size > SIZE_WARN) {
        // eslint-disable-next-line no-console
        console.warn(
          `[sveltego:llms-txt] llms-full.txt is ${(size / 1024).toFixed(1)} KiB — exceeds ${SIZE_WARN / 1024} KiB warning threshold`,
        );
      }
    },
  };
}
