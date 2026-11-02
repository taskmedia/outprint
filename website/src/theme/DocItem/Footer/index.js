import React, {useCallback, useEffect, useRef, useState} from 'react';
import Footer from '@theme-original/DocItem/Footer';
import {useLocation} from '@docusaurus/router';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import {useDocsVersion} from '@docusaurus/plugin-content-docs/client';
import {buildPermalink, copyText} from '../../../lib/permalink';
import styles from './styles.module.css';

const CONFIRMATION_MS = 2500;

export default function FooterWrapper(props) {
  const {siteConfig} = useDocusaurusContext();
  const version = useDocsVersion();
  const {pathname, hash} = useLocation();
  const [status, setStatus] = useState('idle');
  const timer = useRef(undefined);

  const permalinkFor = useCallback(
    (anchor) =>
      buildPermalink({
        siteUrl: siteConfig.url,
        baseUrl: siteConfig.baseUrl,
        pathname,
        hash: anchor,
        versionName: version.version,
        isLast: version.isLast,
      }),
    [siteConfig, pathname, version],
  );

  const copy = useCallback(
    async (anchor) => {
      const copied = await copyText(permalinkFor(anchor));
      setStatus(copied ? 'copied' : 'failed');
      clearTimeout(timer.current);
      timer.current = setTimeout(() => setStatus('idle'), CONFIRMATION_MS);
    },
    [permalinkFor],
  );

  // The # link next to a heading navigates to the unpinned location on the
  // newest version; also put the pinned URL on the clipboard.
  useEffect(() => {
    const onClick = (event) => {
      const link = event.target.closest?.('a.hash-link');
      if (link) copy(new URL(link.href).hash);
    };
    document.addEventListener('click', onClick);
    return () => document.removeEventListener('click', onClick);
  }, [copy]);

  useEffect(() => () => clearTimeout(timer.current), []);

  return (
    <>
      <Footer {...props} />
      <div className={styles.permalink}>
        <button
          type="button"
          className={styles.button}
          onClick={() => copy(hash)}>
          Copy link to this version
        </button>
        <span role="status" className={styles.status}>
          {status === 'copied' && 'Link copied'}
          {status === 'failed' && 'Copy failed, use the address bar'}
        </span>
      </div>
    </>
  );
}
