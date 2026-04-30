<script module>
  export const snapshot = {
    capture: () => window.scrollY,
    restore: (y) => window.scrollTo(0, y),
  };
</script>
<script lang="go">
func helper(x int) string { return "x" }
</script>
<p>{helper(1)}</p>
