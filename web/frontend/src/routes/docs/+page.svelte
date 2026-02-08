<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api/client';

  interface DocItem {
    id: string;
    title: string;
    description: string;
  }

  interface DocContent {
    id: string;
    title: string;
    content: string;
  }

  let docs: DocItem[] = [];
  let selectedDoc: DocContent | null = null;
  let loading = true;
  let error = '';

  onMount(async () => {
    try {
      docs = await api.get('/docs');
    } catch (e) {
      error = 'Failed to load documentation';
    } finally {
      loading = false;
    }
  });

  async function loadDoc(id: string) {
    try {
      selectedDoc = await api.get(`/docs/${id}`);
    } catch (e) {
      error = 'Failed to load document';
    }
  }

  function renderMarkdown(content: string): string {
    // Simple markdown rendering - converts basic markdown to HTML
    return content
      // Headers
      .replace(/^### (.*$)/gim, '<h3>$1</h3>')
      .replace(/^## (.*$)/gim, '<h2>$1</h2>')
      .replace(/^# (.*$)/gim, '<h1>$1</h1>')
      // Bold
      .replace(/\*\*(.*)\*\*/gim, '<strong>$1</strong>')
      // Italic
      .replace(/\*(.*)\*/gim, '<em>$1</em>')
      // Code blocks
      .replace(/```(\w*)\n([\s\S]*?)```/gim, '<pre><code class="language-$1">$2</code></pre>')
      // Inline code
      .replace(/`([^`]+)`/gim, '<code>$1</code>')
      // Links
      .replace(/\[([^\]]+)\]\(([^)]+)\)/gim, '<a href="$2" target="_blank">$1</a>')
      // Tables - basic support (track if we've seen a header)
      .replace(/\|(.+)\|/gim, (match, _, offset, string) => {
        const cells = match.split('|').filter(c => c.trim());
        // Skip separator rows (rows with only dashes and colons)
        if (cells.every(c => c.match(/^[-:\s]+$/))) {
          return '';
        }
        // Check if this is the first row (header) by seeing if previous line doesn't have |
        const prevNewline = string.lastIndexOf('\n', offset - 1);
        const prevLine = prevNewline >= 0 ? string.substring(prevNewline + 1, offset).trim() : '';
        const isFirstRow = !prevLine.includes('|');
        const tag = isFirstRow ? 'th' : 'td';
        const cellHtml = cells.map(c => `<${tag}>${c.trim()}</${tag}>`).join('');
        return `<tr>${cellHtml}</tr>`;
      })
      // Horizontal rules
      .replace(/^---$/gim, '<hr>')
      // Line breaks
      .replace(/\n/gim, '<br>');
  }
</script>

<div class="page-header">
  <h1>Documentation</h1>
</div>

{#if error}
  <div class="alert alert-error">{error}</div>
{/if}

<div class="docs-container">
  <aside class="docs-sidebar">
    <h3>Documents</h3>
    {#if loading}
      <p>Loading...</p>
    {:else}
      <ul>
        {#each docs as doc}
          <li class:active={selectedDoc?.id === doc.id}>
            <button on:click={() => loadDoc(doc.id)}>
              <strong>{doc.title}</strong>
              <span>{doc.description}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </aside>

  <main class="docs-content">
    {#if selectedDoc}
      <div class="doc-viewer">
        {@html renderMarkdown(selectedDoc.content)}
      </div>
    {:else}
      <div class="docs-welcome">
        <h2>📖 TapeBackarr Documentation</h2>
        <p>Select a document from the sidebar to view its contents.</p>
        
        <div class="quick-links">
          <h3>Quick Links</h3>
          <ul>
            <li><strong>Usage Guide</strong> - Complete guide to using TapeBackarr</li>
            <li><strong>Operator Guide</strong> - Quick reference for daily operations</li>
            <li><strong>Manual Recovery</strong> - How to recover data without the application</li>
            <li><strong>API Reference</strong> - REST API documentation</li>
          </ul>
        </div>
      </div>
    {/if}
  </main>
</div>

<style>
  .alert {
    padding: 1rem;
    border-radius: 8px;
    margin-bottom: 1rem;
  }

  .alert-error {
    background: var(--badge-danger-bg);
    color: var(--badge-danger-text);
    border: 1px solid var(--accent-danger);
  }

  .docs-container {
    display: flex;
    gap: 2rem;
    min-height: 70vh;
  }

  .docs-sidebar {
    width: 280px;
    flex-shrink: 0;
    background: var(--bg-card);
    border-radius: 12px;
    padding: 1.5rem;
    box-shadow: var(--shadow);
  }

  .docs-sidebar h3 {
    margin: 0 0 1rem;
    font-size: 1rem;
    color: var(--text-secondary);
  }

  .docs-sidebar ul {
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .docs-sidebar li {
    margin-bottom: 0.5rem;
  }

  .docs-sidebar li.active button {
    background: #4a4aff;
    color: white;
  }

  .docs-sidebar button {
    width: 100%;
    padding: 0.75rem;
    border: none;
    border-radius: 8px;
    background: var(--bg-input);
    cursor: pointer;
    text-align: left;
    transition: all 0.2s ease;
  }

  .docs-sidebar button:hover {
    background: var(--bg-card-hover);
  }

  .docs-sidebar button strong {
    display: block;
    font-size: 0.875rem;
  }

  .docs-sidebar button span {
    display: block;
    font-size: 0.75rem;
    color: var(--text-muted);
    margin-top: 0.25rem;
  }

  .docs-sidebar li.active button span {
    color: rgba(255, 255, 255, 0.8);
  }

  .docs-content {
    flex: 1;
    background: var(--bg-card);
    border-radius: 12px;
    padding: 2rem;
    box-shadow: var(--shadow);
    overflow-x: auto;
  }

  .docs-welcome {
    text-align: center;
    padding: 3rem;
  }

  .docs-welcome h2 {
    font-size: 1.5rem;
    margin-bottom: 0.5rem;
  }

  .docs-welcome p {
    color: var(--text-secondary);
    margin-bottom: 2rem;
  }

  .quick-links {
    text-align: left;
    max-width: 400px;
    margin: 0 auto;
  }

  .quick-links h3 {
    font-size: 1rem;
    margin-bottom: 1rem;
    color: var(--text-secondary);
  }

  .quick-links ul {
    list-style: none;
    padding: 0;
  }

  .quick-links li {
    padding: 0.5rem 0;
    border-bottom: 1px solid var(--border-color);
  }

  .doc-viewer {
    line-height: 1.6;
  }

  .doc-viewer :global(h1) {
    font-size: 2rem;
    margin-bottom: 1rem;
    padding-bottom: 0.5rem;
    border-bottom: 2px solid #4a4aff;
  }

  .doc-viewer :global(h2) {
    font-size: 1.5rem;
    margin: 2rem 0 1rem;
    color: #333;
  }

  .doc-viewer :global(h3) {
    font-size: 1.25rem;
    margin: 1.5rem 0 0.75rem;
    color: #555;
  }

  .doc-viewer :global(pre) {
    background: #f5f6fa;
    padding: 1rem;
    border-radius: 8px;
    overflow-x: auto;
  }

  .doc-viewer :global(code) {
    font-family: 'Fira Code', 'Monaco', monospace;
    font-size: 0.875rem;
  }

  .doc-viewer :global(table) {
    width: 100%;
    border-collapse: collapse;
    margin: 1rem 0;
  }

  .doc-viewer :global(th),
  .doc-viewer :global(td) {
    padding: 0.75rem;
    text-align: left;
    border: 1px solid #ddd;
  }

  .doc-viewer :global(th) {
    background: #f5f6fa;
  }

  .doc-viewer :global(a) {
    color: #4a4aff;
  }

  .doc-viewer :global(hr) {
    border: none;
    border-top: 1px solid #eee;
    margin: 2rem 0;
  }
</style>
