<!-- sveltego:ssr-fallback -->
<script lang="ts">
  import { updated } from '$app/state';

  async function checkNow() {
    await updated.check();
  }
</script>

<h1>updated rune fixture</h1>
<p data-testid="updated-current">updated.current = {String(updated.current)}</p>
<button data-testid="check-now" type="button" onclick={checkNow}>updated.check()</button>
<p>
  This page exercises the SvelteKit-equivalent <code>updated</code> rune. The
  background poller fetches <code>/_app/version.json</code> on the server's
  configured cadence and flips the value above to <code>true</code> when the
  build version drifts. Click the button to force an immediate check.
</p>
