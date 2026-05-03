<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<h1>{data.title}</h1>
<p>{data.body}</p>
<a href="/post/1" data-testid="link-post-1">Post 1</a>
<a href="/post/2" data-testid="link-post-2">Post 2</a>
<a href="/" data-testid="link-home">Back</a>
