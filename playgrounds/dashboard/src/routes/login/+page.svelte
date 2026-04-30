<script lang="go"></script>

<div class="max-w-3xl mx-auto px-4 py-8">
  <nav class="flex items-center gap-4 pb-3 mb-6 border-b border-gray-200">
    <a href="/" class="text-blue-600 hover:underline text-sm font-medium">Home</a>
    <a href="/login" class="text-blue-600 hover:underline text-sm font-medium">Login</a>
  </nav>

  <h1 class="text-2xl font-bold text-gray-900 mb-6">Sign in</h1>

  {#if data.LastError != ""}
    <p class="text-red-600 text-sm mb-4">{data.LastError}</p>
  {/if}

  <form method="post" action="/login?/default" class="space-y-4 max-w-sm">
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Username
        <input
          type="text"
          name="username"
          value={data.LastUsername}
          required
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        >
      </label>
    </div>
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Password
        <input
          type="password"
          name="password"
          required
          class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        >
      </label>
    </div>
    <button
      type="submit"
      class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 cursor-pointer"
    >
      Sign in
    </button>
  </form>

  <p class="mt-6 text-sm text-gray-500">Demo credentials: <code class="bg-gray-100 px-1 py-0.5 rounded text-xs">admin</code> / <code class="bg-gray-100 px-1 py-0.5 rounded text-xs">password123</code>.</p>
</div>
