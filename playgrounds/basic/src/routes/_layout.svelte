<script lang="ts">
  let { children } = $props();
  // Layout-scoped $state survives cross-route same-chain SPA navs (#518)
  // when the chainKey-shared wrapper instance is reused. The button
  // double-doubles as a quick smoke check from the basic playground.
  let count = $state(0);
</script>

<header data-testid="layout-header">
  <button data-testid="layout-count-inc" onclick={() => count++}>+</button>
  <span data-testid="layout-count-value">{count}</span>
</header>

{@render children()}
