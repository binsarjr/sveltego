<script lang="go">
	let count = $state(3)
	let doubled = $derived(count * 2)
</script>
<p>{doubled}</p>
