import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Select, Spin } from 'antd';
import type { SelectProps, DefaultOptionType } from 'antd/es/select';
import { debounce } from '@/lib/utils/debounce';

export interface RemoteSelectOption<T = unknown> {
  value: string | number;
  label: React.ReactNode;
  data?: T;
}

export interface RemoteSelectProps<
  ValueType extends string | number = number,
  OptionData = unknown,
> extends Omit<SelectProps<ValueType>, 'options' | 'optionRender'> {
  fetchOptions: (search: string) => Promise<RemoteSelectOption<OptionData>[]>;
  fetchPage?: (search: string, cursor?: string) => Promise<{ options: RemoteSelectOption<OptionData>[]; nextCursor?: string }>;
  fetchByValue?: (value: ValueType) => Promise<RemoteSelectOption<OptionData> | null>;
  debounceTimeout?: number;
  optionRender?: (option: RemoteSelectOption<OptionData>) => React.ReactNode;
  fetchOnDropdownOpen?: boolean;
  /** Bump this when the tenant/permission scope changes so old options cannot survive it. */
  scopeKey?: string;
}

const toKey = (value: string | number) => `${value}`;

const RemoteSelect = <
  ValueType extends string | number = number,
  OptionData = unknown,
>({
  fetchOptions,
  fetchPage,
  fetchByValue,
  debounceTimeout = 300,
  optionRender,
  fetchOnDropdownOpen = true,
  scopeKey,
  notFoundContent,
  onOpenChange,
  value,
  mode,
  ...restProps
}: RemoteSelectProps<ValueType, OptionData>) => {
  const [searchOptions, setSearchOptions] = useState<RemoteSelectOption<OptionData>[]>([]);
  const [globalOptions, setGlobalOptions] = useState<
    Record<string, RemoteSelectOption<OptionData>>
  >({});
  const [fetching, setFetching] = useState(false);
  const [nextCursor, setNextCursor] = useState<string>();
  const [loadError, setLoadError] = useState(false);
  const searchRef = useRef('');
  const fetchIdRef = useRef(0);

  const mergeOptions = useCallback((incoming: RemoteSelectOption<OptionData>[]) => {
    if (!incoming?.length) return;

    setGlobalOptions((prev) => {
      const next = { ...prev };
      incoming.forEach((item) => {
        next[toKey(item.value)] = item;
      });
      return next;
    });
  }, []);

  const loadOptions = useCallback(
    async (keyword: string, forceImmediate = false) => {
      const triggerFetch = async () => {
        fetchIdRef.current += 1;
        const currentFetchId = fetchIdRef.current;
        setFetching(true);
        try {
          const page = fetchPage ? await fetchPage(keyword) : { options: await fetchOptions(keyword) };
          const result = page.options;
          if (fetchIdRef.current !== currentFetchId) return;
          searchRef.current = keyword;
          setSearchOptions(result);
          setNextCursor(page.nextCursor);
          setLoadError(false);
          mergeOptions(result);
        } catch {
          if (fetchIdRef.current === currentFetchId) {
            setSearchOptions([]);
            setNextCursor(undefined);
            setLoadError(true);
          }
        } finally {
          if (fetchIdRef.current === currentFetchId) {
            setFetching(false);
          }
        }
      };

      if (forceImmediate) {
        triggerFetch();
        return;
      }

      triggerFetch();
    },
    [fetchOptions, fetchPage, mergeOptions],
  );

  const loadNextPage = useCallback(async () => {
    if (!fetchPage || !nextCursor || fetching) return;
    const currentFetchId = ++fetchIdRef.current;
    setFetching(true);
    try {
      const page = await fetchPage(searchRef.current, nextCursor);
      if (currentFetchId !== fetchIdRef.current) return;
      setSearchOptions((previous) => {
        const merged = new Map(previous.map((item) => [toKey(item.value), item]));
        page.options.forEach((item) => merged.set(toKey(item.value), item));
        return Array.from(merged.values());
      });
      mergeOptions(page.options);
      setNextCursor(page.nextCursor);
      setLoadError(false);
    } catch {
      if (currentFetchId === fetchIdRef.current) setLoadError(true);
    } finally {
      if (currentFetchId === fetchIdRef.current) setFetching(false);
    }
  }, [fetchPage, fetching, mergeOptions, nextCursor]);

  const debouncedFetcher = useMemo(
    () =>
      debounce((keyword: string) => {
        loadOptions(keyword);
      }, debounceTimeout),
    [loadOptions, debounceTimeout],
  );

  useEffect(() => () => debouncedFetcher.cancel(), [debouncedFetcher]);

  useEffect(() => {
    fetchIdRef.current += 1;
    searchRef.current = '';
    setSearchOptions([]);
    setGlobalOptions({});
    setNextCursor(undefined);
    setLoadError(false);
  }, [scopeKey]);

  const extractValues = useCallback(
    (selectValue: SelectProps<ValueType>['value']) => {
      if (selectValue == null) {
        return [] as (string | number)[];
      }

      if (restProps.labelInValue) {
        const labeledValues = Array.isArray(selectValue) ? selectValue : [selectValue];
        return labeledValues
          .map((item) =>
            typeof item === 'object' && item !== null && 'value' in item
              ? item.value
              : undefined,
          )
          .filter((item): item is string | number =>
            typeof item === 'string' || typeof item === 'number',
          );
      }

      if (Array.isArray(selectValue)) {
        return (selectValue as (string | number)[]).filter(
          (val) => val !== undefined && val !== null,
        );
      }

      return [selectValue as string | number];
    },
    [restProps.labelInValue],
  );

  useEffect(() => {
    if (!fetchByValue) return;

    const selectedValues = extractValues(value);
    if (!selectedValues.length) return;

    const missingValues = selectedValues.filter((val) => !globalOptions[toKey(val)]);
    if (!missingValues.length) return;

    let mounted = true;
    setFetching(true);

    Promise.all(
      missingValues.map(async (val) => {
        try {
          const option = await fetchByValue(val as ValueType);
          return option ?? undefined;
        } catch {
          return undefined;
        }
      }),
    )
      .then((result) => {
        if (!mounted) return;
        const validOptions = result.filter(
          (item): item is RemoteSelectOption<OptionData> => !!item,
        );
        mergeOptions(validOptions);
      })
      .finally(() => {
        if (mounted) {
          setFetching(false);
        }
      });

    return () => {
      mounted = false;
    };
  }, [value, fetchByValue, extractValues, globalOptions, mergeOptions]);

  const optionsToRender: DefaultOptionType[] = useMemo(() => {
    const selectedValues = extractValues(value);
    const selectedOptions = selectedValues
      .map((val) => globalOptions[toKey(val)])
      .filter((opt): opt is RemoteSelectOption<OptionData> => !!opt);

    const mergedMap = new Map<string, RemoteSelectOption<OptionData>>();
    [...selectedOptions, ...searchOptions].forEach((item) => {
      mergedMap.set(toKey(item.value), item);
    });

    return Array.from(mergedMap.values()).map((item) => ({
      value: item.value,
      label: item.label,
      // Keep the option metadata at the same level as the public
      // RemoteSelectOption contract.  Nesting the whole option here made
      // custom renderers receive `{ data: { value, label, data } }`, which
      // silently dropped policy metadata in the review profile picker.
      data: item.data,
    }));
  }, [extractValues, value, globalOptions, searchOptions]);

  const handleSearch = useCallback(
    (keyword: string) => {
      debouncedFetcher(keyword.trim());
    },
    [debouncedFetcher],
  );

  const handleDropdownVisibleChange = useCallback(
    (open: boolean) => {
      if (open && fetchOnDropdownOpen) {
        loadOptions('', true);
      }
      onOpenChange?.(open);
    },
    [fetchOnDropdownOpen, loadOptions, onOpenChange],
  );

  const handlePopupScroll = useCallback((event: React.UIEvent<HTMLDivElement>) => {
    const target = event.currentTarget;
    if (target.scrollHeight - target.scrollTop - target.clientHeight < 48) void loadNextPage();
    restProps.onPopupScroll?.(event);
  }, [loadNextPage, restProps]);

  return (
    <Select
      showSearch
      filterOption={false}
      onSearch={handleSearch}
      value={value}
      mode={mode}
      notFoundContent={
        notFoundContent ?? (fetching ? <Spin size="small" /> : loadError ? '加载失败，请重新搜索' : '未找到数据')
      }
      onPopupScroll={handlePopupScroll}
      onOpenChange={handleDropdownVisibleChange}
      options={optionsToRender}
      optionLabelProp="label"
      optionRender={
        optionRender
          ? (option) =>
              optionRender(option.data as RemoteSelectOption<OptionData>)
          : undefined
      }
      {...restProps}
    />
  );
};

export default RemoteSelect;
