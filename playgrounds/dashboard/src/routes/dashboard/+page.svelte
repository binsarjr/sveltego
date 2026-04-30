<script lang="go"></script>

<nav>
  <a href="/">Home</a>
  <a href="/dashboard">Dashboard</a>
  <form method="post" action="/?/logout" style="display:inline;margin:0">
    <button type="submit" class="secondary">Logout ({data.Username})</button>
  </form>
</nav>

<h1>Dashboard</h1>

<p>Signed in as <strong>{data.Username}</strong>.</p>

{#if data.FlashMsg != ""}
  <p class="error">{data.FlashMsg}</p>
{/if}

<h2>Items</h2>

<form method="post" action="/dashboard?/create">
  <input type="text" name="title" placeholder="New item title" required>
  <input type="text" name="note" placeholder="Note">
  <button type="submit">Create</button>
</form>

{#if len(data.Items) > 0}
  <table>
    <thead>
      <tr>
        <th>ID</th>
        <th>Title</th>
        <th>Note</th>
        <th>Updated</th>
        <th></th>
      </tr>
    </thead>
    <tbody>
      {#each data.Items as it}
        <tr>
          <td><code>{it.ID}</code></td>
          <td><a href={"/dashboard/items/" + it.ID}>{it.Title}</a></td>
          <td>{it.Note}</td>
          <td>{it.UpdatedAt}</td>
          <td>
            <form method="post" action={"/dashboard?/delete"} style="display:inline;margin:0">
              <input type="hidden" name="id" value={it.ID}>
              <button type="submit" class="danger">Delete</button>
            </form>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{:else}
  <p>No items yet. Create one above.</p>
{/if}

<h2>Live metrics (auto-refreshes every 5s)</h2>

<meta http-equiv="refresh" content="5;url=/dashboard">

<div class="chart">
  <p style="margin:0 0 .5rem 0">Latest sample: <strong>{data.MetricLatest}</strong> @ <code>{data.MetricLatestTS}</code></p>
  {#each data.MetricBars as b}
    <div><span class="bar" style={b.Width}></span><code>{b.Label}</code> {b.Value}</div>
  {/each}
</div>

<p style="color:#666;font-size:.85rem">JSON endpoint: <code>GET /api/metrics</code> (returns the same series; ready for client-side polling once <a href="https://github.com/binsarjr/sveltego/issues/34">#34</a> ships).</p>
