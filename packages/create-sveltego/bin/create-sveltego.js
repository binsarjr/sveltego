#!/usr/bin/env node
// create-sveltego CLI entry. Loads the compiled wrapper from dist/.
import { main } from '../dist/index.js';

main(process.argv.slice(2)).catch((err) => {
  process.stderr.write(`create-sveltego: ${err?.message ?? err}\n`);
  process.exit(1);
});
