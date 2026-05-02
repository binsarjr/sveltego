<script lang="ts">
  let { children } = $props();
</script>

<!-- Level 2 layout: nested under level1. -->
<section data-layout="2">layout 2</section>
{@render children()}
<section data-layout="2">end 2</section>
