import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://docs.octarq.org',
  base: '/',
  integrations: [
    starlight({
      title: 'Octarq',
      description: 'The self-hosted operations backend for indie hackers and small AI-native teams.',
      social: {
        github: 'https://github.com/octarq-org/octarq',
      },
      sidebar: [
        {
          label: 'Start',
          items: [
            {
              label: 'Overview',
              link: '/',
            },
            {
              label: 'Quickstart',
              link: '/quickstart/',
            },
            {
              label: 'Deploy',
              link: '/deploy/',
            },
          ],
        },
        {
          label: 'Build a Plugin',
          items: [
            {
              label: 'Writing a Plugin',
              link: '/writing-a-plugin/',
            },
            {
              label: 'Plugin Directory',
              link: '/plugin-directory/',
            },
          ],
        },
        {
          label: 'Core Features',
          items: [
            { label: 'Short links', link: '/core/short-links/' },
            { label: 'Mailboxes', link: '/core/mailboxes/' },
            { label: 'DNS', link: '/core/dns/' },
            { label: 'MCP server', link: '/core/mcp/' },
            { label: 'Notifications', link: '/core/notifications/' },
            { label: 'API tokens', link: '/core/api-tokens/' },
          ],
        },
        {
          label: 'Architecture',
          items: [
            {
              label: 'Overview',
              link: '/architecture/overview/',
            },
            {
              label: 'plugin.Context',
              link: '/architecture/plugin-context/',
            },
            {
              label: 'Composition',
              link: '/architecture/composition/',
            },
            {
              label: 'Core Plugins',
              link: '/architecture/core-plugins/',
            },
          ],
        },
        {
          label: 'Guides',
          items: [
            {
              label: 'Publishing',
              link: '/guides/publishing/',
            },
            {
              label: 'Accessibility',
              link: '/guides/accessibility/',
            },
          ],
        },
      ],
    }),
  ],
});
