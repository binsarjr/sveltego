<script lang="go"></script>

<h1 class="text-3xl font-bold text-gray-900 dark:text-gray-100 mb-6">Posts</h1>

{#if len(data.Posts) == 0}
  <p class="text-gray-500 dark:text-gray-400">No posts on this page.</p>
{:else}
  <ul class="divide-y divide-gray-200 dark:divide-gray-700">
    {#each data.Posts as p}
      <li class="py-4">
        <a href={"/" + p.Slug} class="text-lg font-medium text-indigo-600 dark:text-indigo-400 hover:underline">{p.Title}</a>
        <small class="ml-2 text-sm text-gray-500 dark:text-gray-400"> — {p.Summary}</small>
      </li>
    {/each}
  </ul>
{/if}

<nav class="mt-8 flex items-center gap-4">
  {#if data.HasPrev}
    <a href={data.PrevHref} class="px-4 py-2 rounded-md bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700 text-sm font-medium">prev</a>
  {/if}
  <span class="text-sm text-gray-500 dark:text-gray-400"> page {data.Page} of {data.TotalPages} </span>
  {#if data.HasNext}
    <a href={data.NextHref} class="px-4 py-2 rounded-md bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700 text-sm font-medium">next</a>
  {/if}
</nav>
