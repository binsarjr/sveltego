<script lang="go"></script>

<nav>
  <a href="/">Home</a>
  <a href="/login">Login</a>
</nav>

<h1>Sign in</h1>

{#if data.LastError != ""}
  <p class="error">{data.LastError}</p>
{/if}

<form method="post" action="/login?/default">
  <p>
    <label>Username
      <input type="text" name="username" value={data.LastUsername} required>
    </label>
  </p>
  <p>
    <label>Password
      <input type="password" name="password" required>
    </label>
  </p>
  <button type="submit">Sign in</button>
</form>

<p style="color:#666;font-size:.9rem">Demo credentials: <code>admin</code> / <code>password123</code>.</p>
