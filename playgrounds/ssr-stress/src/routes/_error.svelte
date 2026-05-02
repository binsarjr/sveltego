<script lang="ts">
  let { data } = $props();
</script>

<h1>SSR-stress error: {data.code}</h1>
<p>{data.message}</p>
{#if data.id}
  <p>id={data.id}</p>
{/if}
