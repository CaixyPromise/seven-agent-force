'use client';

import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import type { DockerWorkbenchTabKey } from '../types';

interface LegacyDockerRedirectProps {
  tab: DockerWorkbenchTabKey;
}

export function LegacyDockerRedirect({ tab }: LegacyDockerRedirectProps) {
  const navigate = useNavigate();

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    params.set('tab', tab);
    navigate(`/system/docker?${params.toString()}`, { replace: true });
  }, [navigate, tab]);

  return null;
}
