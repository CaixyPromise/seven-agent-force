'use client';

import { useEffect, useRef } from 'react';
import { useConfigValue } from '@/hooks/config';
import { configAssetStablePathOrEmpty } from '@/lib/configAssets';

/**
 * Applies only the reviewed favicon configuration. The value must be the
 * server-issued CONFIG_ASSET path; arbitrary URL schemes, query strings and
 * browser-generated blob/data URLs are ignored before touching the DOM.
 */
export default function RuntimeBrandAssets() {
  const favicon = useConfigValue<string>('SEVEN_FRONTEND_METADATA.favicon');
  const originalHref = useRef<string | undefined>(undefined);
  const safeFaviconPath = configAssetStablePathOrEmpty(favicon?.value);

  useEffect(() => {
    let link = document.querySelector<HTMLLinkElement>('link[rel~="icon"]');
    if (!link) {
      link = document.createElement('link');
      link.rel = 'icon';
      document.head.appendChild(link);
    }
    if (originalHref.current === undefined) {
      originalHref.current = link.getAttribute('href') || '/favicon.ico';
    }
    link.setAttribute('href', safeFaviconPath || originalHref.current);
  }, [safeFaviconPath]);

  return null;
}
