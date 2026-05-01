<script lang="ts">
  import type { PageData } from './_page.svelte';
  let { data }: { data: PageData } = $props();
</script>

<div class="max-w-3xl mx-auto px-4 py-8">
  <nav class="flex items-center gap-4 pb-3 mb-6 border-b border-gray-200">
    <a href="/" class="text-blue-600 hover:underline text-sm font-medium">Home</a>
    <a href="/dashboard" class="text-blue-600 hover:underline text-sm font-medium">Dashboard</a>
    <form method="post" action="/?/logout" class="inline m-0">
      <button type="submit" class="bg-white text-blue-600 border border-blue-600 rounded-md px-3 py-1 text-sm cursor-pointer hover:bg-blue-50">
        Logout ({data.username})
      </button>
    </form>
  </nav>

  <h1 class="text-2xl font-bold text-gray-900 mb-4">Edit item</h1>

  {#if data.flashMsg !== ''}
    <p class="text-red-600 text-sm mb-4">{data.flashMsg}</p>
  {/if}

  <div class="text-sm text-gray-600 mb-4 space-y-1">
    <p><strong class="font-medium text-gray-700">ID:</strong> <code class="bg-gray-100 px-1 py-0.5 rounded text-xs">{data.item.id}</code></p>
    <p><strong class="font-medium text-gray-700">Last updated:</strong> {data.item.updatedAt}</p>
  </div>

  <form method="post" action={'/dashboard/items/' + data.item.id + '?/update'} class="space-y-4 max-w-sm mb-6">
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Title
        <input
          type="text"
          name="title"
          value={data.item.title}
          required
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        >
      </label>
    </div>
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Note
        <input
          type="text"
          name="note"
          value={data.item.note}
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        >
      </label>
    </div>
    <button
      type="submit"
      class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 cursor-pointer"
    >
      Save
    </button>
  </form>

  <form method="post" action={'/dashboard/items/' + data.item.id + '?/delete'}>
    <button
      type="submit"
      class="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 cursor-pointer"
    >
      Delete
    </button>
  </form>

  <p class="mt-6"><a href="/dashboard" class="text-blue-600 hover:underline text-sm">Back to dashboard</a></p>
</div>
