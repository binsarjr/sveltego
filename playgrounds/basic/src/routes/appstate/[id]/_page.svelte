<!-- sveltego:ssr-fallback -->
<script lang="ts">
  import { page, navigating, updated } from '$app/state';
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
  const errorMessage = page.error ? page.error.message : 'null';
  const navigatingType = navigating.current ? navigating.current.type : 'idle';
  const stateKeys = Object.keys(page.state).join(',') || 'none';
  const formLabel = page.form === null ? 'null' : 'set';
  const routeId = page.route.id ?? 'null';
  const paramId = page.params.id ?? 'null';
</script>

<h1>{data.greeting}</h1>

<section data-testid="page-state">
  <p data-field="url">url: {page.url.pathname}</p>
  <p data-field="route">route.id: {routeId}</p>
  <p data-field="param-id">params.id: {paramId}</p>
  <p data-field="status">status: {page.status}</p>
  <p data-field="error">error: {errorMessage}</p>
  <p data-field="data-greeting">data.greeting: {data.greeting}</p>
  <p data-field="form">form: {formLabel}</p>
  <p data-field="state-keys">state keys: {stateKeys}</p>
</section>

<section data-testid="nav-state">
  <p data-field="navigating">navigating: {navigatingType}</p>
  <p data-field="updated">updated: {updated.current ? 'yes' : 'no'}</p>
</section>

<p><a href="/appstate/2">go to /appstate/2</a></p>
<p><a href="/">home</a></p>
