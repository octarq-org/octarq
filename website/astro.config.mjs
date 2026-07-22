import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://docs.octarq.com',
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
          label: 'Overview',
          link: '/',
        },
        {
          label: 'Quickstart',
          link: '/quickstart/',
        },
        {
          label: 'Writing a Plugin',
          link: '/writing-a-plugin/',
        },
        {
          label: 'Deploy',
          link: '/deploy/',
        },
        {
          label: 'Plugin Directory',
          link: '/plugin-directory/',
        },
      ],
    }),
  ],
});
