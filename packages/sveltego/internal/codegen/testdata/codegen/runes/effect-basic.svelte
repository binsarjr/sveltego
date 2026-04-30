<script lang="go">
	let count = $state(0)
	$effect(func() { _ = count })
</script>
<p>{count}</p>
