'use client';

import { GithubOutlined } from '@ant-design/icons';
import { Typography } from 'antd';
import React from 'react';

const { Link, Text } = Typography;

const Footer: React.FC = () => {
  const currentYear = new Date().getFullYear();

  return (
    <footer
      style={{
        display: 'grid',
        gap: 6,
        justifyItems: 'center',
        padding: '20px 16px 8px',
        color: '#0f172a',
      }}
    >
      <Text style={{ color: '#0f172a', fontSize: 13 }}>
        {currentYear} © Seven-Framework
      </Text>
      <Link
        href="https://www.github.com/CaixyPromise"
        target="_blank"
        rel="noopener noreferrer"
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: 8,
          color: '#0f172a',
          fontSize: 13,
          fontWeight: 500,
        }}
      >
        <GithubOutlined />
        <span>CaixyPromise</span>
      </Link>
    </footer>
  );
};

export default Footer;
