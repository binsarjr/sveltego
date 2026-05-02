<script>
	import { page, navigating } from '$app/state';
	import { goto } from '$app/navigation';
	let { data } = $props();
	const target = '/named-import';
	function nav() {
		goto(target);
	}
</script>

<h1>{data.greeting}</h1>
<p>{page.url.pathname}</p>
<p>{navigating.current ? 'navigating' : 'idle'}</p>
<button onclick={nav}>go</button>
