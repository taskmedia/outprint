// @ts-check

const baseUrl = '/outprint/';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Outprint',
  tagline:
    'Outprint for Outline: turn Outline documents into typeset PDFs (letters, reports, custom LaTeX templates) with a Download PDF button. Self-hosted, no Outline fork.',
  favicon: 'brand/favicon.ico',

  url: 'https://taskmedia.github.io',
  baseUrl,
  organizationName: 'taskmedia',
  projectName: 'outprint',

  // GitHub Pages serves page/index.html at /page/ and redirects /page to
  // /page/. Using the slash form everywhere avoids that redirect and keeps
  // one canonical URL per page, which the version-correct links from the
  // product (/<minor>/errors/<code>/) rely on.
  trailingSlash: true,

  // Strict: a broken link, anchor or markdown link fails the build.
  onBrokenLinks: 'throw',
  onBrokenAnchors: 'throw',

  markdown: {
    // The docs are plain CommonMark for GitHub; 'detect' keeps .md files out
    // of MDX so braces and angle brackets in prose do not break the build.
    format: 'detect',
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },

  headTags: [
    {tagName: 'link', attributes: {rel: 'icon', href: `${baseUrl}brand/favicon.svg`, type: 'image/svg+xml'}},
    {tagName: 'link', attributes: {rel: 'apple-touch-icon', href: `${baseUrl}brand/apple-touch-icon.png`}},
  ],

  // One locale. To add German: see website/README.md ("Add a locale").
  // Do not add the locale dropdown navbar item before a second locale exists.
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          path: '../docs',
          routeBasePath: '/',
          sidebarPath: './sidebars.js',
          editUrl: 'https://github.com/taskmedia/outprint/edit/main/docs/',
          // Repo-only pages, not published.
          exclude: [
            '**/_*.{js,jsx,ts,tsx,md,mdx}',
            '**/_*/**',
            '**/*.test.{js,jsx,ts,tsx}',
            '**/__tests__/**',
            'sanitize.md',
            'publishing.md',
            'releasing.md',
            'testing.md',
            'research/**',
            // The logo kit (build script, lock files, its own README).
            'assets/**',
          ],
          // Until the first release snapshot exists, the current docs are the
          // only version and stay at the site root. At the first snapshot:
          // see website/README.md ("What to change at the first snapshot").
          // Do NOT set lastVersion.
          // versions: {current: {label: 'dev', path: 'dev', banner: 'unreleased'}},
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themes: [
    [
      '@easyops-cn/docusaurus-search-local',
      /** @type {import('@easyops-cn/docusaurus-search-local').PluginOptions} */
      ({
        hashed: true,
        indexDocs: true,
        indexBlog: false,
        indexPages: false,
        docsRouteBasePath: '/',
        docsDir: '../docs',
        language: 'en',
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      colorMode: {
        defaultMode: 'light',
        respectPrefersColorScheme: true,
      },
      image: 'brand/social-preview.png',
      navbar: {
        // The lockup already carries the name.
        logo: {
          alt: 'Outprint',
          src: 'brand/outprint-lockup.svg',
          srcDark: 'brand/outprint-lockup-dark.svg',
        },
        items: [
          // At the first snapshot add: {type: 'docsVersionDropdown', position: 'right'}
          {
            href: 'https://github.com/taskmedia/outprint',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'light',
        copyright: 'Outprint for Outline. Not affiliated with Outline.',
      },
      docs: {
        sidebar: {autoCollapseCategories: false},
      },
      prism: {
        theme: require('prism-react-renderer').themes.github,
        darkTheme: require('prism-react-renderer').themes.vsDark,
        additionalLanguages: ['bash', 'yaml', 'json', 'latex', 'docker'],
      },
      tableOfContents: {minHeadingLevel: 2, maxHeadingLevel: 3},
    }),
};

module.exports = config;
