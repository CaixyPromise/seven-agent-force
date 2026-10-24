interface SystemLayoutProps {
  children: React.ReactNode;
}

export default function SystemLayout({ children }: SystemLayoutProps) {
  return (
    <div className="system-layout">
      {children}
    </div>
  );
}
