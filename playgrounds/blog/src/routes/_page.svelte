<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<h1 class="text-3xl font-bold text-gray-900 dark:text-gray-100 mb-6">Posts</h1>

{#if data.posts.length === 0}
  <p class="text-gray-500 dark:text-gray-400">No posts on this page.</p>
{:else}
  <ul class="divide-y divide-gray-200 dark:divide-gray-700">
    {#each data.posts as p}
      <li class="py-4">
        <a href={'/' + p.slug} class="text-lg font-medium text-indigo-600 dark:text-indigo-400 hover:underline">{p.title}</a>
        <small class="ml-2 text-sm text-gray-500 dark:text-gray-400"> — {p.summary}</small>
      </li>
    {/each}
  </ul>
{/if}

<nav class="mt-8 flex items-center gap-4">
  {#if data.hasPrev}
    <a href={data.prevHref} class="px-4 py-2 rounded-md bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700 text-sm font-medium">prev</a>
  {/if}
  <span class="text-sm text-gray-500 dark:text-gray-400"> page {data.page} of {data.totalPages} </span>
  {#if data.hasNext}
    <a href={data.nextHref} class="px-4 py-2 rounded-md bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700 text-sm font-medium">next</a>
  {/if}
</nav>
