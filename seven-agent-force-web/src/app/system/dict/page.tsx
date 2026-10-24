'use client';

import React, { useState } from 'react';
import { DictProvider } from './context/DictContext';
import { DictTypeSidebar } from './components/DictTypeSidebar';
import { DictItemList } from './components/DictItemList';
import { CreateDictTypeModal } from './components/CreateDictTypeModal';

/**
 * 字典管理主页面
 */
const DictManagePage: React.FC = () => {
  const [isModalOpen, setIsModalOpen] = useState(false);

  return (
    <DictProvider>
      <div className="flex h-[calc(100vh-120px)] min-h-[560px] gap-4 overflow-hidden p-4 bg-gray-50">
        <DictTypeSidebar onCreateClick={() => setIsModalOpen(true)} />
        <DictItemList />
        <CreateDictTypeModal open={isModalOpen} onCancel={() => setIsModalOpen(false)} />
      </div>
    </DictProvider>
  );
};

export default DictManagePage;
