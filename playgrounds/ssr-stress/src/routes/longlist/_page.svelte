<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<h1>{data.title}</h1>
<ul>
  {#each data.items as item}
    <li>{item}</li>
  {/each}
</ul>
