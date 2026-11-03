import React from 'react';
import Link from '@docusaurus/Link';
import Layout from '@theme/Layout';
import ThemedImage from '@theme/ThemedImage';
import useBaseUrl from '@docusaurus/useBaseUrl';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';

import styles from './index.module.css';

const METADATA_EXAMPLE = [
  ['type', 'letter'],
  ['subject', 'Spring plant sale'],
  ['reference', 'PS-2026-03'],
  ['recipient', null],
  ['', '  - Ada Example'],
  ['', '  - 1 Sample Lane'],
  ['', '  - 12345 Exampleton'],
  ['closing', 'With best wishes'],
];

const STEPS = [
  {
    title: 'Pick a Template',
    text: 'A Template provides the look and structure: a letter, a report, or your own design in LaTeX.',
  },
  {
    title: 'Bind it to a Collection',
    text: 'A Collection admin pairs the Template with a Configuration that holds the organisation name, logo and colours.',
  },
  {
    title: 'Write and download',
    text: 'Authors fill a few fields at the top of a Document and click Download PDF.',
  },
];

const FEATURES = [
  {
    title: 'No Outline fork',
    text: 'A small reverse proxy adds the Download PDF button. Outline itself stays untouched and upgradable.',
  },
  {
    title: 'Real typesetting',
    text: 'Pandoc and LuaLaTeX turn the Document into a PDF, so letters and reports look like they were set, not exported.',
  },
  {
    title: 'Content apart from branding',
    text: 'Names, logos and colours live in the Configuration. Authors write content and cannot override them.',
  },
  {
    title: 'Renders in a sandbox',
    text: 'The Worker that runs LaTeX has no network and no secrets. It only turns a bundle into a PDF.',
  },
];

// The sections of the docs. docs/README.md keeps the same list as a
// plain table for GitHub.
const SECTIONS = [
  {to: '/write/', title: 'Write', line: 'Make a PDF from a document, and fix a message.'},
  {to: '/install/', title: 'Install', line: 'Install Outprint next to your own Outline.'},
  {to: '/configure/', title: 'Configure', line: 'Give a Collection its PDF look.'},
  {to: '/templates/', title: 'Templates', line: 'Browse the catalogue, or make your own.'},
  {to: '/reference/', title: 'Reference', line: 'Every key, field and message, one by one.'},
];

function DocumentCard() {
  return (
    <figure className={styles.document} aria-label="Example Metadata Block in an Outline document">
      <figcaption className={styles.documentCaption}>Top of an Outline document</figcaption>
      <pre className={styles.documentCode}>
        <code>
          <span className={styles.fence}>```yaml</span>
          {'\n'}
          {METADATA_EXAMPLE.map(([key, value], index) => (
            <React.Fragment key={index}>
              {key ? <span className={styles.key}>{key}:</span> : null}
              {value ? <span className={styles.value}>{key ? ' ' : ''}{value}</span> : null}
              {'\n'}
            </React.Fragment>
          ))}
          <span className={styles.fence}>```</span>
          {'\n'}
          <span className={styles.body}>Thank you for your enquiry ...</span>
        </code>
      </pre>
    </figure>
  );
}

export default function Home() {
  const {siteConfig} = useDocusaurusContext();
  const sections = SECTIONS.filter(
    (section) => section.title !== 'Reference' || siteConfig.customFields.rootHasReference,
  ).map((section) =>
    section.title === 'Install' ? {...section, to: siteConfig.customFields.installPath} : section,
  );
  return (
    <Layout
      title="PDFs from Outline documents"
      description="Turn Outline documents into typeset PDFs with a Download PDF button. Self-hosted, no Outline fork.">
      <header className={styles.hero}>
        <div className={styles.heroInner}>
          <div className={styles.heroText}>
            <h1 className={styles.banner}>
              <ThemedImage
                alt="Outprint: PDFs from Outline documents"
                sources={{
                  light: useBaseUrl('/brand/outprint-banner.svg'),
                  dark: useBaseUrl('/brand/outprint-banner-dark.svg'),
                }}
              />
            </h1>
            <p className={styles.lead}>
              Turn <a href="https://www.getoutline.com/">Outline</a> documents into typeset PDFs: letters,
              reports or your own LaTeX design. Authors write in Outline and click{' '}
              <strong>Download PDF</strong>. You host it yourself, and Outline is not forked.
            </p>
            <div className={styles.actions}>
              <Link className={styles.primary} to="/write/">
                Read the documentation
              </Link>
              <Link className={styles.secondary} href="https://github.com/taskmedia/outprint">
                View on GitHub
              </Link>
            </div>
          </div>
          <DocumentCard />
        </div>
      </header>

      <main>
        <section className={styles.section} aria-labelledby="how-heading">
          <h2 id="how-heading" className={styles.sectionTitle}>
            From Outline document to PDF in three steps
          </h2>
          <ol className={styles.steps}>
            {STEPS.map((step) => (
              <li key={step.title} className={styles.step}>
                <h3>{step.title}</h3>
                <p>{step.text}</p>
              </li>
            ))}
          </ol>
        </section>

        <section className={styles.section} aria-labelledby="why-heading">
          <h2 id="why-heading" className={styles.sectionTitle}>
            Built to be run by a small team
          </h2>
          <dl className={styles.features}>
            {FEATURES.map((feature) => (
              <div key={feature.title} className={styles.feature}>
                <dt>{feature.title}</dt>
                <dd>{feature.text}</dd>
              </div>
            ))}
          </dl>
        </section>

        <section className={styles.section} aria-labelledby="next-heading">
          <h2 id="next-heading" className={styles.sectionTitle}>
            Find your way in
          </h2>
          <nav aria-label="Documentation sections">
            <ol className={styles.list}>
              {sections.map(({to, title, line}, index) => (
                <li key={to} className={styles.item}>
                  <Link to={to} className={styles.row}>
                    <span className={styles.number} aria-hidden="true">
                      {String(index + 1).padStart(2, '0')}
                    </span>
                    <span className={styles.word}>{title}</span>
                    <span className={styles.line}>{line}</span>
                    <span className={styles.arrow} aria-hidden="true">
                      →
                    </span>
                  </Link>
                </li>
              ))}
            </ol>
          </nav>
        </section>
      </main>
    </Layout>
  );
}
