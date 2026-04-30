import { glob } from 'tinyglobby';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import path from 'node:path';
import type { Plugin } from 'vite';

function stripFrontmatter(src: string): string {
  if (!src.startsWith('---\n')) return src;
  const end = src.indexOf('\n---\n', 4);
  if (end === -1) return src;
  return src.slice(end + 5).replace(/^\n+/, '');
}

export default function rawMarkdownPlugin(): Plugin {
  let outDir = '';
  let docsRoot = '';
  return {
    name: 'sveltego:raw-md',
    configResolved(cfg) {
      outDir = cfg.build.outDir;
      docsRoot = cfg.root;
    },
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        if (!req.url) return next();
        const u = req.url.split('?')[0];
        if (!u.endsWith('.md')) return next();
        const rel = u.replace(/^\//, '');
        try {
          const src = await readFile(path.join(server.config.root, rel), 'utf8');
          res.setHeader('Content-Type', 'text/markdown; charset=utf-8');
          res.end(stripFrontmatter(src));
        } catch {
          next();
        }
      });
    },
    async closeBundle() {
      if (!outDir || !docsRoot) return;
      const files = await glob(['**/*.md', '!.vitepress/**', '!node_modules/**'], {
        cwd: docsRoot,
      });
      for (const file of files) {
        const src = await readFile(path.join(docsRoot, file), 'utf8');
        const out = path.join(outDir, file);
        await mkdir(path.dirname(out), { recursive: true });
        await writeFile(out, stripFrontmatter(src));
      }
    },
  };
}
