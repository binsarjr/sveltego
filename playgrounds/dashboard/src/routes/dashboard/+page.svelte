<script lang="go"></script>

<div class="max-w-4xl mx-auto px-4 py-8">
  <nav class="flex items-center gap-4 pb-3 mb-6 border-b border-gray-200">
    <a href="/" class="text-blue-600 hover:underline text-sm font-medium">Home</a>
    <a href="/dashboard" class="text-blue-600 hover:underline text-sm font-medium">Dashboard</a>
    <form method="post" action="/?/logout" class="inline m-0">
      <button type="submit" class="bg-white text-blue-600 border border-blue-600 rounded-md px-3 py-1 text-sm cursor-pointer hover:bg-blue-50">
        Logout ({data.Username})
      </button>
    </form>
  </nav>

  <h1 class="text-2xl font-bold text-gray-900 mb-2">Dashboard</h1>
  <p class="text-gray-600 mb-6">Signed in as <strong class="font-semibold text-gray-900">{data.Username}</strong>.</p>

  {#if data.FlashMsg != ""}
    <p class="text-red-600 text-sm mb-4">{data.FlashMsg}</p>
  {/if}

  <h2 class="text-lg font-semibold text-gray-800 mb-3">Items</h2>

  <form method="post" action="/dashboard?/create" class="flex gap-2 mb-6">
    <input
      type="text"
      name="title"
      placeholder="New item title"
      required
      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
    >
    <input
      type="text"
      name="note"
      placeholder="Note"
      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
    >
    <button
      type="submit"
      class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 cursor-pointer"
    >
      Create
    </button>
  </form>

  {#if len(data.Items) > 0}
    <div class="overflow-x-auto mb-8">
      <table class="w-full border-collapse text-sm">
        <thead>
          <tr class="border-b border-gray-200">
            <th class="text-left py-2 px-3 font-medium text-gray-600">ID</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">Title</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">Note</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">Updated</th>
            <th class="py-2 px-3"></th>
          </tr>
        </thead>
        <tbody>
          {#each data.Items as it}
            <tr class="border-b border-gray-100 hover:bg-gray-50">
              <td class="py-2 px-3"><code class="text-xs bg-gray-100 px-1 py-0.5 rounded">{it.ID}</code></td>
              <td class="py-2 px-3"><a href={"/dashboard/items/" + it.ID} class="text-blue-600 hover:underline">{it.Title}</a></td>
              <td class="py-2 px-3 text-gray-600">{it.Note}</td>
              <td class="py-2 px-3 text-gray-500 text-xs">{it.UpdatedAt}</td>
              <td class="py-2 px-3">
                <form method="post" action={"/dashboard?/delete"} class="inline m-0">
                  <input type="hidden" name="id" value={it.ID}>
                  <button
                    type="submit"
                    class="rounded-md bg-red-600 px-3 py-1 text-xs font-medium text-white hover:bg-red-700 cursor-pointer"
                  >
                    Delete
                  </button>
                </form>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {:else}
    <p class="text-gray-500 text-sm mb-8">No items yet. Create one above.</p>
  {/if}

  <h2 class="text-lg font-semibold text-gray-800 mb-3">Live metrics <span class="text-sm font-normal text-gray-500">(auto-refreshes every 5s)</span></h2>

  <meta http-equiv="refresh" content="5;url=/dashboard">

  <div class="border border-dashed border-gray-300 rounded-lg p-4 bg-gray-50 font-mono text-sm mb-4">
    <p class="mb-3 text-gray-700">Latest sample: <strong class="text-gray-900">{data.MetricLatest}</strong> @ <code class="text-xs bg-gray-100 px-1 py-0.5 rounded">{data.MetricLatestTS}</code></p>
    {#each data.MetricBars as b}
      <div class="flex items-center gap-2 mb-1">
        <span class="inline-block h-4 bg-blue-600 rounded-sm" style={b.Width}></span>
        <code class="text-xs text-gray-600">{b.Label}</code>
        <span class="text-xs text-gray-500">{b.Value}</span>
      </div>
    {/each}
  </div>

  <p class="text-xs text-gray-400">JSON endpoint: <code class="bg-gray-100 px-1 py-0.5 rounded">GET /api/metrics</code> (returns the same series; ready for client-side polling once <a href="https://github.com/binsarjr/sveltego/issues/34" class="text-blue-600 hover:underline">#34</a> ships).</p>
</div>
