'use client';

import { PropsWithChildren, useState } from 'react';
import {
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query';
import { ReactQueryDevtools } from '@tanstack/react-query-devtools';

const defaultOptions = {
  queries: {
    refetchOnWindowFocus: false,
    retry: 1,
  },
};

export default function ReactQueryProvider({ children }: PropsWithChildren) {
  const [client] = useState(() => new QueryClient({ defaultOptions }));

  return (
    <QueryClientProvider client={client}>
      {children}
      {import.meta.env.DEV ? (
        <ReactQueryDevtools initialIsOpen={false} />
      ) : null}
    </QueryClientProvider>
  );
}
