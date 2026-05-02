<script lang="ts">
  let { children } = $props();
</script>

<!-- Inert in MVP. Layout chain rendering lands in v0.2 (#24). -->
{@render children()}
