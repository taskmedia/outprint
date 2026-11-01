import React from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import styles from './index.module.css';

// The five sections of the docs (issue #52). One word, one line each.
// docs/README.md keeps the same list as a plain table for GitHub.
const sections = [
  {to: '/write/', title: 'Write', line: 'Make a PDF from a document, and fix a message.'},
  {to: '/configure/', title: 'Configure', line: 'Give a Collection its PDF look.'},
  {to: '/deploy/', title: 'Deploy', line: 'Install Outprint next to your own Outline.'},
  {to: '/templates/', title: 'Templates', line: 'Browse the catalogue, or make your own.'},
  {to: '/internals/', title: 'Internals', line: 'How it works, and why it is safe.'},
];

export default function Home() {
  return (
    <Layout
      title="Outprint for Outline"
      description="Outprint for Outline: turn Outline documents into typeset PDFs (letters, reports, custom LaTeX templates) with a Download PDF button. Self-hosted, no Outline fork.">
      <main className={styles.page}>
        <header className={styles.intro}>
          <p className={styles.kicker}>Documentation</p>
          <h1 className={styles.title}>
            Outline documents,
            <br />
            typeset as PDF.
          </h1>
          <p className={styles.lead}>
            Outprint adds a Download PDF button to your own Outline. Letters and
            reports get a letterhead, a footer and a fixed layout. Nobody has to
            design anything.
          </p>
        </header>
        <nav className={styles.sections} aria-label="Documentation sections">
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
      </main>
    </Layout>
  );
}
