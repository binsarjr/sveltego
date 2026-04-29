{#if Data.HasItems}
  <ul>
    {#each Data.Items as item, i}
      <li>{i}: {item.Name}</li>
    {/each}
  </ul>
{:else}
  <p>none</p>
{/if}