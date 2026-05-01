<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<article class="prose dark:prose-invert max-w-none">
  <h1 class="text-3xl font-bold text-gray-900 dark:text-gray-100 mb-2">{data.title}</h1>
  <p class="text-sm text-gray-500 dark:text-gray-400 mb-6"><small>{data.date}</small></p>
  <div class="text-gray-700 dark:text-gray-300 leading-relaxed">
    {@html data.html}
  </div>
</article>

<section class="mt-12">
  <h2 class="text-2xl font-semibold text-gray-900 dark:text-gray-100 mb-6">Comments ({data.comments.length})</h2>

  {#if data.comments.length === 0}
    <p class="text-gray-500 dark:text-gray-400 mb-6">No comments yet. Be the first.</p>
  {:else}
    <ul class="divide-y divide-gray-200 dark:divide-gray-700 mb-8">
      {#each data.comments as c}
        <li class="py-4">
          <div class="flex items-baseline gap-2 mb-1">
            <strong class="font-semibold text-gray-900 dark:text-gray-100">{c.author}</strong>
            <small class="text-xs text-gray-500 dark:text-gray-400"> — {c.posted}</small>
          </div>
          <p class="text-gray-700 dark:text-gray-300">{c.body}</p>
        </li>
      {/each}
    </ul>
  {/if}

  <form method="post" class="mt-6 space-y-4">
    {#if data.form != null}
      <p class="text-sm text-red-600 dark:text-red-400">comment was rejected — please fill in name and body.</p>
    {/if}
    <div>
      <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
        name
        <input name="author" required class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-indigo-500" />
      </label>
    </div>
    <div>
      <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
        comment
        <textarea name="body" required rows="3" class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"></textarea>
      </label>
    </div>
    <button type="submit" class="px-4 py-2 rounded-md bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-700 dark:hover:bg-indigo-500 transition-colors">post comment</button>
  </form>
</section>

<p class="mt-8"><a href="/" class="text-indigo-600 dark:text-indigo-400 hover:underline text-sm">back to index</a></p>
