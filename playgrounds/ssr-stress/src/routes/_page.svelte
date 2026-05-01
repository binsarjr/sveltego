<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<h1>{data.greeting}</h1>
<p>SSR stress fixtures: <a href="/longlist">longlist</a>, <a href="/conditional">conditional</a>, <a href="/layoutdeep/level2/level3">layoutdeep</a>.</p>
