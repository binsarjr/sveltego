<script lang="go"></script>

<nav>
  <a href="/">Home</a>
  {#if data.LoggedIn}
    <a href="/dashboard">Dashboard</a>
    <form method="post" action="/?/logout" style="display:inline;margin:0">
      <button type="submit" class="secondary">Logout ({data.Username})</button>
    </form>
  {:else}
    <a href="/login">Login</a>
  {/if}
</nav>

<h1>sveltego dashboard</h1>

{#if data.LoggedIn}
  <p>Welcome back, <strong>{data.Username}</strong>.</p>
  <p><a href="/dashboard">Go to your dashboard.</a></p>
{:else}
  <p>This playground demonstrates cookie-session authentication, CRUD via
  form actions, and a JSON polling endpoint. Sign in to continue.</p>
  <p><a href="/login">Log in</a> to view the dashboard.</p>
{/if}
