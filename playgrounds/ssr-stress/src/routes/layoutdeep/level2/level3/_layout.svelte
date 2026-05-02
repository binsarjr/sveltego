<script lang="ts">
  let { children } = $props();
</script>

<!-- Level 3 layout: innermost wrapper. -->
<aside data-layout="3">layout 3</aside>
{@render children()}
<aside data-layout="3">end 3</aside>
