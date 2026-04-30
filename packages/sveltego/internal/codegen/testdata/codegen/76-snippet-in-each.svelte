{#snippet row(item string)}<li>{item}</li>{/snippet}<ul>{#each Items as it}{@render row(it)}{/each}</ul>
