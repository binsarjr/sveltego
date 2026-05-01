<script lang="go"></script>

<div class="max-w-3xl mx-auto px-4 py-8">
  <nav class="flex items-center gap-4 pb-3 mb-6 border-b border-gray-200">
    <a href="/" class="text-blue-600 hover:underline text-sm font-medium">Home</a>
    {#if data.LoggedIn}
      <a href="/dashboard" class="text-blue-600 hover:underline text-sm font-medium">Dashboard</a>
      <form method="post" action="/?/logout" class="inline m-0">
        <button type="submit" class="bg-white text-blue-600 border border-blue-600 rounded-md px-3 py-1 text-sm cursor-pointer hover:bg-blue-50">
          Logout ({data.Username})
        </button>
      </form>
    {:else}
      <a href="/login" class="text-blue-600 hover:underline text-sm font-medium">Login</a>
    {/if}
  </nav>

  <h1 class="text-2xl font-bold text-gray-900 mb-4">sveltego dashboard</h1>

  {#if data.LoggedIn}
    <p class="text-gray-700 mb-2">Welcome back, <strong class="font-semibold">{data.Username}</strong>.</p>
    <p class="text-gray-700"><a href="/dashboard" class="text-blue-600 hover:underline">Go to your dashboard.</a></p>
  {:else}
    <p class="text-gray-600 mb-3">This playground demonstrates cookie-session authentication, CRUD via
    form actions, and a JSON polling endpoint. Sign in to continue.</p>
    <p class="text-gray-700"><a href="/login" class="text-blue-600 hover:underline font-medium">Log in</a> to view the dashboard.</p>
  {/if}
</div>
