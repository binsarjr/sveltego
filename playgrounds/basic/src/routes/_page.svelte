<!-- sveltego:ssr-fallback -->
<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<h1>{data.greeting}</h1>
<p>Welcome to the sveltego basic playground.</p>
<p><a href="/post/1">Read post 1</a></p>
