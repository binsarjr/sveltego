<script lang="ts">
  import type { ErrorData } from './_error.svelte';
  let { data }: { data: ErrorData } = $props();
</script>

<h1>Error {data.code}</h1>
<p>{data.message}</p>
{#if data.id}
  <p>Trace ID: <code>{data.id}</code></p>
{/if}
<p><a href="/">Back home</a></p>
