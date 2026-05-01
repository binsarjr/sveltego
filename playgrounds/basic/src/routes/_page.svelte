<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<h1>{data.greeting}</h1>

{#if data.posts && data.posts.length > 0}
  <ul>
    {#each data.posts as post}
      <li><a href={'/post/' + post.id}>{post.title}</a></li>
    {/each}
  </ul>
{:else}
  <p>No posts.</p>
{/if}
