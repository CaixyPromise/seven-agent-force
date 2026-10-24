import AppFrame from './AppFrame';

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return <AppFrame>{children}</AppFrame>;
}
