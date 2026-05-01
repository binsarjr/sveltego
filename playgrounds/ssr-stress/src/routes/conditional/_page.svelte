<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<h1>conditional</h1>
{#if data.loggedIn}
  <p>Welcome back, {data.username}.</p>
{:else}
  <p>Please <a href="/login">log in</a>.</p>
{/if}
