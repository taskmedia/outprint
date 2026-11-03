// @ts-check

const fs = require('fs');
const path = require('path');

// The released minors, newest first, written by scripts/docs-versions.mjs from
// the Git tags before every build. Absent until the first tag vX.Y.0.
const versionsFile = path.join(__dirname, 'versions.json');
const releasedVersions = fs.existsSync(versionsFile)
  ? JSON.parse(fs.readFileSync(versionsFile, 'utf8'))
  : [];
const hasRelease = releasedVersions.length > 0;

// The product links to /<minor>/... for a release and to /dev/... for a
// development build (internal/docslink). Docusaurus serves the newest
// release at the site root and the unreleased docs at /dev/, so the paths the
// product uses for those two do not exist as folders. They are redirects,
// created here from the same versions.json, so they are right from the first
// release on:
//   with a release:     /<newest minor>/*  ->  /*
//   without a release:  /dev/*             ->  /*   (the current docs are the root)
const rootAlias = hasRelease ? releasedVersions[0] : 'dev';
const versionFolders = ['dev', ...releasedVersions];

// The Install section was called Deploy until the release that is served at the
// site root has it (issue #114). The navbar and the landing page link to the
// section under the name the root version has.
const rootHasInstall =
  !hasRelease ||
  fs.existsSync(path.join(__dirname, 'versioned_docs', `version-${releasedVersions[0]}`, 'install'));
const installPath = rootHasInstall ? '/install/' : '/deploy/';

// The Reference section is new (issue #119). Until the release served at the
// root has it, the navbar and the landing page do not link to it.
const rootHasReference =
  !hasRelease ||
  fs.existsSync(path.join(__dirname, 'versioned_docs', `version-${releasedVersions[0]}`, 'reference'));

/** @param {string} existingPath */
function createRedirects(existingPath) {
  if (existingPath.endsWith('.html')) return undefined;
  const inVersionFolder = versionFolders.some(
    (v) => existingPath === `/${v}` || existingPath.startsWith(`/${v}/`),
  );
  if (inVersionFolder) return undefined;
  return [`/${rootAlias}${existingPath}`];
}

// Deploy pages that moved into folders (issue #97) and then into the Install
// section (issue #114). The old address of each page redirects to the new one.
// The PDF-by-link and command-line pages are gone and have no redirect.
// Only the current docs moved: a released
// version keeps the layout it was tagged with, so its pages stay real pages
// and are not redirected. The current docs live at /dev/ once a release exists,
// at the root before; in that case /dev/... is also a redirect target the
// product may link to, so both spellings are covered.
const movedDeployPages = {
  'way-menu': 'ways/export-menu',
  'docker-compose': 'platforms/docker-compose',
  'helm-kubernetes': 'platforms/helm-kubernetes',
  injector: 'components/injector',
  'image-fetching': 'components/image-fetching',
  'limits-and-admission': 'components/limits-and-admission',
  'reverse-proxy-recipes': 'components/reverse-proxy-recipes',
  'security-for-operators': 'operate/security-for-operators',
  upgrading: 'operate/upgrading',
  compatibility: 'operate/compatibility',
};
const currentDocsPath = hasRelease ? '/dev' : '';
const movedPageRedirects = Object.entries(movedDeployPages).flatMap(([from, to]) => {
  const target = `${currentDocsPath}/install/${to}/`;
  const sources = [`${currentDocsPath}/deploy/${from}/`];
  if (!hasRelease) sources.push(`/dev/deploy/${from}/`);
  return sources.map((source) => ({from: source, to: target}));
});

// The site has its own domain (GitHub Pages custom domain, see
// website/README.md), so it lives at the root.
const baseUrl = '/';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Outprint',
  tagline:
    'Outprint for Outline: turn Outline documents into typeset PDFs (letters, reports, custom LaTeX templates) with a Download PDF button. Self-hosted, no Outline fork.',
  favicon: 'brand/favicon.ico',
  customFields: {installPath, rootHasReference},

  url: 'https://outprint.task.media',
  baseUrl,
  organizationName: 'taskmedia',
  projectName: 'outprint',

  // GitHub Pages serves page/index.html at /page/ and redirects /page to
  // /page/. Using the slash form everywhere avoids that redirect and keeps
  // one canonical URL per page, which the version-correct links from the
  // product (/<minor>/reference/error-messages/#<code>) rely on.
  trailingSlash: true,

  // Strict: a broken link, anchor or markdown link fails the build.
  onBrokenLinks: 'throw',
  onBrokenAnchors: 'throw',

  markdown: {
    // The docs are plain CommonMark for GitHub; 'detect' keeps .md files out
    // of MDX so braces and angle brackets in prose do not break the build.
    format: 'detect',
    // Diagrams are ```mermaid fences, which GitHub renders as well.
    mermaid: true,
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
          // A section's intro page (README.md) is the link of its category,
          // so it must not show up a second time as the first child.
          async sidebarItemsGenerator({defaultSidebarItemsGenerator, ...args}) {
            const items = await defaultSidebarItemsGenerator(args);
            return items.filter(
              (item) => !(item.type === 'doc' && item.id.endsWith('/README')),
            );
          },
          editUrl: 'https://github.com/taskmedia/outprint/edit/main/docs/',
          // Repo-only pages, not published. docs/README.md is the plain table
          // for GitHub; the site's landing page is src/pages/index.js.
          exclude: [
            'README.md',
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
          // Until the first release tag exists, the current docs are the
          // only version and stay at the site root; with a release they move to
          // /dev/ and the newest release takes the root. Do NOT set lastVersion
          // (see website/README.md, "Versions").
          versions: hasRelease
            ? {current: {label: 'dev', path: 'dev', banner: 'unreleased'}}
            : undefined,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  plugins: [
    [
      '@docusaurus/plugin-client-redirects',
      {createRedirects, redirects: movedPageRedirects},
    ],
  ],

  themes: [
    '@docusaurus/theme-mermaid',
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
          {to: '/write/', label: 'Write', position: 'left', activeBaseRegex: '^/write/'},
          {to: installPath, label: 'Install', position: 'left', activeBaseRegex: `^${installPath}`},
          {to: '/configure/', label: 'Configure', position: 'left', activeBaseRegex: '^/configure/'},
          {to: '/templates/', label: 'Templates', position: 'left', activeBaseRegex: '^/templates/'},
          ...(rootHasReference
            ? [{to: '/reference/', label: 'Reference', position: 'left', activeBaseRegex: '^/reference/'}]
            : []),
          ...(hasRelease ? [{type: 'docsVersionDropdown', position: 'right'}] : []),
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
      // Diagrams are drawn at their natural size (16 px text, like the body)
      // and scroll sideways when they are wider than the column. The default,
      // useMaxWidth, shrinks the whole canvas to the column, which made the
      // text 26 to 61 % of its size. The font is the site's, so Mermaid
      // measures labels with the face that is shown.
      mermaid: {
        options: {
          fontFamily: "Roboto, system-ui, -apple-system, 'Segoe UI', sans-serif",
          flowchart: {useMaxWidth: false, wrappingWidth: 160},
          sequence: {useMaxWidth: false, wrap: true, width: 100, actorMargin: 16, mirrorActors: false},
        },
      },
    }),
};

module.exports = config;
