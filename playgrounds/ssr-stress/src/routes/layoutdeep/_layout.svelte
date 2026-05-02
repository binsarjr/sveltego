<script lang="ts">
  let { children } = $props();
</script>

<!-- Level 1 layout: top-level wrapper. Content rendering is gated on
     issue #439 (children-callback ABI). Today this acts as a slot
     pass-through under SSR; once #439 lands, the surrounding markup
     should appear in the rendered body. -->
<header data-layout="1">layout 1</header>
{@render children()}
<footer data-layout="1">end 1</footer>
