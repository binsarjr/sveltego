<script setup lang="ts">
import { ref } from 'vue';
import { useData } from 'vitepress';

const { page, frontmatter } = useData();
const toast = ref('');

function pageTitle(): string {
  return (
    (frontmatter.value && (frontmatter.value as Record<string, string>).title) ||
    page.value.title ||
    page.value.relativePath
  );
}

function pageURL(): string {
  if (typeof window === 'undefined') return '';
  return window.location.href;
}

function rawMdURL(): string {
  if (typeof window === 'undefined') return '';
  const rel = page.value.relativePath;
  return new URL(`/${rel}`, window.location.origin).toString();
}

async function fetchMd(): Promise<string> {
  const res = await fetch(rawMdURL());
  if (!res.ok) throw new Error(`fetch ${res.status}`);
  return await res.text();
}

async function copy(forLLM: boolean) {
  try {
    let body = await fetchMd();
    if (forLLM) {
      body = `# sveltego docs — ${pageTitle()}\n\nSource: ${pageURL()}\n\n${body}`;
    }
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(body);
    } else {
      const ta = document.createElement('textarea');
      ta.value = body;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
    }
    toast.value = 'Copied';
  } catch (err) {
    toast.value = `Failed: ${(err as Error).message}`;
  }
  setTimeout(() => {
    toast.value = '';
  }, 1800);
}
</script>

<template>
  <div class="copy-for-llm">
    <button type="button" @click="copy(false)" aria-label="Copy page as Markdown">
      Copy as Markdown
    </button>
    <button type="button" @click="copy(true)" aria-label="Copy page for LLM">
      Copy for LLM
    </button>
    <span v-if="toast" class="toast">{{ toast }}</span>
  </div>
</template>
