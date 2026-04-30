import { defineConfig } from 'vitepress';
import llmsTxt from './plugins/llms-txt';
import rawMarkdown from './plugins/raw-md';

export default defineConfig({
  title: 'sveltego',
  description: 'SvelteKit-shape framework for Go. SSR via codegen, no JS server runtime.',
  cleanUrls: true,
  lastUpdated: true,
  appearance: 'dark',
  head: [
    ['meta', { name: 'theme-color', content: '#ff3e00' }],
  ],
  themeConfig: {
    nav: [
      { text: 'Guide', link: '/guide/quickstart' },
      { text: 'Reference', link: '/reference/kit' },
      { text: 'AI', link: '/ai-development' },
      { text: 'GitHub', link: 'https://github.com/binsarjr/sveltego' },
    ],
    sidebar: {
      '/guide/': [
        {
          text: 'Getting started',
          items: [
            { text: 'Quickstart', link: '/guide/quickstart' },
          ],
        },
        {
          text: 'Concepts',
          items: [
            { text: 'Routing', link: '/guide/routing' },
            { text: 'Load', link: '/guide/load' },
            { text: 'Form actions', link: '/guide/actions' },
            { text: 'Hooks', link: '/guide/hooks' },
            { text: 'Errors', link: '/guide/errors' },
            { text: 'Components', link: '/guide/components' },
            { text: 'Snippets', link: '/guide/snippets' },
          ],
        },
        {
          text: 'Build & deploy',
          items: [
            { text: 'Build', link: '/guide/build' },
            { text: 'Deploy', link: '/guide/deploy' },
            { text: 'CSP', link: '/guide/csp' },
          ],
        },
        {
          text: 'Migration',
          items: [
            { text: 'From SvelteKit', link: '/guide/migration' },
            { text: 'FAQ', link: '/guide/faq' },
          ],
        },
      ],
      '/reference/': [
        {
          text: 'Reference',
          items: [
            { text: 'kit', link: '/reference/kit' },
            { text: 'Routes', link: '/reference/routes' },
            { text: 'Manifest', link: '/reference/manifest' },
            { text: 'CLI', link: '/reference/cli' },
          ],
        },
      ],
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/binsarjr/sveltego' },
    ],
    search: {
      provider: 'local',
    },
    editLink: {
      pattern: 'https://github.com/binsarjr/sveltego/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
    footer: {
      message: 'Released under the MIT License.',
      copyright: 'sveltego contributors',
    },
  },
  vite: {
    plugins: [llmsTxt(), rawMarkdown()],
  },
  markdown: {
    theme: { light: 'github-light', dark: 'github-dark' },
    lineNumbers: false,
  },
});
