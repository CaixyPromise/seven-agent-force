'use client';

import React from 'react';
import { Card, Input, Select, DatePicker, Button } from 'antd';
import { SearchOutlined, FilterOutlined, ClearOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';

const { RangePicker } = DatePicker;

interface FileFilters {
    fileName?: string;
    fileType?: string;
    bizType?: number;
    startTime?: string;
    endTime?: string;
}

interface FileFilterProps {
    filters: FileFilters;
    onChange: (filters: FileFilters) => void;
    onReset: () => void;
}

const fileTypeOptions = [
    { label: '全部类型', value: '' },
    { label: '图片', value: 'image' },
    { label: '视频', value: 'video' },
    { label: '文档', value: 'document' },
    { label: '压缩包', value: 'archive' },
    { label: '其他', value: 'other' },
];

const bizTypeOptions = [
    { label: '全部业务', value: '' },
    { label: '头像', value: 1 },
    { label: '附件', value: 2 },
    { label: '资源', value: 3 },
];

const FileFilter: React.FC<FileFilterProps> = ({ filters, onChange, onReset }) => {
    const handleChange = <K extends keyof FileFilters>(key: K, value: FileFilters[K]) => {
        onChange({ ...filters, [key]: value });
    };

    const handleDateChange = (dates: [Dayjs | null, Dayjs | null] | null) => {
        if (dates) {
            onChange({
                ...filters,
                startTime: dates[0]?.format('YYYY-MM-DD 00:00:00'),
                endTime: dates[1]?.format('YYYY-MM-DD 23:59:59'),
            });
        } else {
            onChange({
                ...filters,
                startTime: undefined,
                endTime: undefined,
            });
        }
    };

    return (
        <Card
            className="mb-4 shadow-sm"
            styles={{ body: { padding: '16px' } }}
        >
            <div className="flex flex-wrap items-center gap-4">
                <div className="flex items-center gap-2 text-gray-500">
                    <FilterOutlined />
                    <span className="font-medium">筛选条件</span>
                </div>

                <Input
                    placeholder="搜索文件名/SHA256..."
                    prefix={<SearchOutlined className="text-gray-400" />}
                    value={filters.fileName}
                    onChange={(e) => handleChange('fileName', e.target.value)}
                    className="w-64"
                    allowClear
                />

                <Select
                    placeholder="文件类型"
                    value={filters.fileType}
                    onChange={(value) => handleChange('fileType', value)}
                    options={fileTypeOptions}
                    className="w-32"
                    allowClear
                />

                <Select
                    placeholder="业务类型"
                    value={filters.bizType}
                    onChange={(value) => handleChange('bizType', value)}
                    options={bizTypeOptions}
                    className="w-32"
                    allowClear
                />

                <RangePicker
                    value={
                        filters.startTime && filters.endTime
                            ? [dayjs(filters.startTime), dayjs(filters.endTime)]
                            : null
                    }
                    onChange={handleDateChange}
                    placeholder={['开始日期', '结束日期']}
                />

                <Button
                    icon={<ClearOutlined />}
                    onClick={onReset}
                >
                    重置
                </Button>
            </div>
        </Card>
    );
};

export default FileFilter;
