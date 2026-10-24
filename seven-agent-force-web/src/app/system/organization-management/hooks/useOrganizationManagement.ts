import { useState, useCallback, useEffect, useRef } from 'react';
import {
  getOrgTree,
  createOrg,
  updateOrg,
  deleteOrg,
  moveOrg,
} from '@/api/sysOrgController';
import {
  getDeptTree,
  createDept,
  updateDept,
  deleteDept,
  getDeptById,
} from '@/api/sysDeptController';
import {
  getPostPage,
  createPost,
  updatePost,
  deletePost,
} from '@/api/sysPostController';
import {
  getRolePage,
} from '@/api/sysRoleController';

// 基础实体类型
interface BaseEntity {
  id: API.Int64;
  name: string;
  code?: string;
  status: number;
  sortOrder: number;
  createTime: string;
  updateTime?: string;
  remark?: string;
}

interface OrganizationEntity extends BaseEntity {
  type: 'org';
  parentId?: string;
}

interface DepartmentEntity extends BaseEntity {
  type: 'dept';
  orgId: string;
  parentId: string;
  leaderUserId?: API.Int64;
  hierarchy?: string;
  level?: number;
}

interface PostEntity extends BaseEntity {
  type: 'post';
  orgId: string;
  deptId: string;
  roleCount?: number;
}

type EntityType = OrganizationEntity | DepartmentEntity | PostEntity;

// 分页参数
interface PageParams {
  current?: number;
  size?: number;
  keyword?: string;
  status?: number;
}

// 部门查询参数
interface DeptPageParams extends PageParams {
  orgId: API.Int64;
  parentId?: API.Int64;
}

// 岗位查询参数
interface PostPageParams extends PageParams {
  orgId: API.Int64;
  deptId: API.Int64;
}

// hooks选项
interface UseOrganizationManagementOptions {
  onSuccess?: (action: string) => void;
  onError?: (error: unknown) => void;
}

/**
 * 组织管理相关的业务逻辑hooks
 */
export const useOrganizationManagement = (options: UseOrganizationManagementOptions = {}) => {
  const [loading, setLoading] = useState(false);
  const optionsRef = useRef(options);

  // 部门树缓存，避免重复请求
  const deptTreeCache = useRef<API.SysDept[] | null>(null);

  useEffect(() => {
    optionsRef.current = options;
  }, [options]);

  // 失效部门树缓存的函数
  const invalidateDeptCache = useCallback(() => {
    console.log('失效部门树缓存');
    deptTreeCache.current = null;
  }, []);

  // 获取组织列表 - 移除回调依赖，避免循环
  const fetchOrganizations = useCallback(async (params: PageParams = {}) => {
    setLoading(true);
    try {
      const response = await getOrgTree();
      // 处理树形数据转换为列表格式
      const organizations: OrganizationEntity[] = (response.data || []).map((org) => ({
          id: String(org.id),
          name: org.name || '',
          code: org.code,
          status: org.status ?? 0,
          sortOrder: org.sortOrder ?? 0,
          createTime: org.createTime || '',
          updateTime: org.updateTime,
          parentId: org.parentId,
          type: 'org' as const,
      }));
      const keyword = params.keyword?.trim().toLowerCase();
      const filteredOrganizations = organizations.filter((org) => {
        const matchesKeyword =
          !keyword ||
          org.name.toLowerCase().includes(keyword) ||
          org.code?.toLowerCase().includes(keyword);
        const matchesStatus = params.status === undefined || org.status === params.status;
        return matchesKeyword && matchesStatus;
      });
      return { data: filteredOrganizations, total: filteredOrganizations.length };
    } catch (error) {
      console.error('获取组织列表失败:', error);
      return { data: [], total: 0 };
    } finally {
      setLoading(false);
    }
  }, []); // 移除 onError 依赖

  // 获取部门列表 - 使用缓存避免重复请求
  const fetchDepartments = useCallback(async (params: DeptPageParams) => {
    setLoading(true);
    try {
      // 使用缓存避免重复请求树形接口
      let response;
      if (!deptTreeCache.current) {
        console.log('首次请求部门树，建立缓存');
        response = await getDeptTree();
        deptTreeCache.current = response.data || [];
      } else {
        console.log('使用缓存的部门树数据');
        response = { data: deptTreeCache.current };
      }

      // 从树形数据中筛选指定组织和父部门的子部门
      const filterDeptsByParent = (
        depts: API.SysDept[],
        orgId: API.Int64,
        parentId: API.Int64,
      ): DepartmentEntity[] => {
        const result: DepartmentEntity[] = [];

        const traverse = (items: API.SysDept[]) => {
          items.forEach(item => {
            // 确保类型转换，API返回的可能是字符串
            const itemOrgId = String(item.orgId);
            const itemParentId = String(item.parentId);

            if (itemOrgId === orgId && itemParentId === parentId) {
              result.push({
                id: String(item.id),
                name: item.name || '',
                code: item.code,
                status: item.status ?? 0,
                sortOrder: item.sortOrder ?? 0,
                createTime: item.createTime || '',
                updateTime: item.updateTime,
                remark: item.remark,
                orgId: itemOrgId,
                parentId: itemParentId,
                leaderUserId: item.leaderUserId,
                hierarchy: item.hierarchy,
                level: item.level,
                type: 'dept' as const,
              });
            }
            if (item.children && item.children.length > 0) {
              traverse(item.children);
            }
          });
        };

        traverse(depts);
        return result;
      };

      const departments = filterDeptsByParent(response.data || [], params.orgId, params.parentId || '0');

      // 添加调试日志
      console.log('fetchDepartments 调试信息:', {
        params,
        responseData: response.data,
        filteredDepartments: departments,
        orgId: params.orgId,
        parentId: params.parentId || '0'
      });

      // 应用搜索和状态筛选
      let filteredDepts = departments;
      if (params.keyword) {
        filteredDepts = filteredDepts.filter(dept =>
          dept.name.includes(params.keyword!) || (dept.code && dept.code.includes(params.keyword!))
        );
      }
      if (params.status !== undefined) {
        filteredDepts = filteredDepts.filter(dept => dept.status === params.status);
      }

      // 模拟分页
      const current = params.current || 1;
      const size = params.size || 10;
      const startIndex = (current - 1) * size;
      const endIndex = startIndex + size;
      const pagedData = filteredDepts.slice(startIndex, endIndex);

      return {
        data: pagedData,
        total: filteredDepts.length,
        current,
        size
      };
    } catch (error) {
      console.error('获取部门列表失败:', error);
      return { data: [], total: 0, current: 1, size: 10 };
    } finally {
      setLoading(false);
    }
  }, []); // 移除 onError 依赖

  // 获取岗位列表 - 移除回调依赖，避免循环
  const fetchPosts = useCallback(async (params: PostPageParams) => {
    setLoading(true);
    try {
      const response = await getPostPage({
        current: params.current || 1,
        size: params.size || 10,
        name: params.keyword,
        status: params.status,
        // 注意：根据实际API参数调整，可能需要在查询结果中过滤orgId和deptId
      });

      // 转换为标准格式并过滤指定部门的岗位
      const posts = (response.data?.records || [])
        .filter((post) => {
          const matchesOrg = String(post.orgId) === params.orgId;
          const matchesDept = String(post.deptId) === params.deptId;
          console.log('岗位过滤:', {
            post: post.name,
            postOrgId: post.orgId,
            postDeptId: post.deptId,
            paramsOrgId: params.orgId,
            paramsDeptId: params.deptId,
            matchesOrg,
            matchesDept
          });
          return matchesOrg && matchesDept;
        })
        .map((post): PostEntity => ({
          id: String(post.id),
          name: post.name || '',
          code: post.code,
          status: post.status ?? 0,
          sortOrder: post.sortOrder ?? 0,
          createTime: post.createTime || '',
          updateTime: post.updateTime,
          remark: post.remark,
          orgId: String(post.orgId),
          deptId: String(post.deptId),
          type: 'post' as const,
        }));

      return {
        data: posts,
        total: Number(response.data?.total) || 0,
        current: Number(response.data?.current) || 1,
        size: Number(response.data?.size) || 10
      };
    } catch (error) {
      console.error('获取岗位列表失败:', error);
      return { data: [], total: 0, current: 1, size: 10 };
    } finally {
      setLoading(false);
    }
  }, []); // 移除 onError 依赖

  // 创建实体 - 移除回调依赖，避免循环
  const createEntity = useCallback(async (entity: Partial<EntityType>) => {
    setLoading(true);
    try {
      switch (entity.type) {
        case 'org':
          await createOrg({
            name: entity.name!,
            code: entity.code,
            status: entity.status || 0,
            sortOrder: entity.sortOrder || 0,
            parentId: entity.parentId || '0',
          });
          invalidateDeptCache(); // 组织创建会影响部门树
          break;
        case 'dept':
          await createDept({
            name: entity.name!,
            code: entity.code,
            orgId: (entity as DepartmentEntity).orgId,
            parentId: (entity as DepartmentEntity).parentId,
            status: entity.status || 0,
            sortOrder: entity.sortOrder || 0,
          });
          invalidateDeptCache(); // 部门创建会影响部门树
          break;
        case 'post':
          await createPost({
            code: entity.code!,
            name: entity.name!,
            status: entity.status || 0,
            sortOrder: entity.sortOrder || 0,
            remark: entity.remark,
            orgId: (entity as PostEntity).orgId,
            deptId: (entity as PostEntity).deptId,
          });
          break;
      }
    } catch (error) {
      console.error('创建失败:', error);
      throw error; // 重新抛出错误，让调用方处理
    } finally {
      setLoading(false);
    }
  }, [invalidateDeptCache]);

  // 更新实体 - 移除回调依赖，避免循环
  const updateEntity = useCallback(async (entity: EntityType) => {
    setLoading(true);
    try {
      switch (entity.type) {
        case 'org':
          await updateOrg({
            id: entity.id,
            name: entity.name,
            code: entity.code,
            status: entity.status,
            sortOrder: entity.sortOrder,
            parentId: entity.parentId || '0',
          });
          invalidateDeptCache(); // 组织更新会影响部门树
          break;
        case 'dept':
          await updateDept({
            id: entity.id,
            name: entity.name,
            code: entity.code,
            orgId: (entity as DepartmentEntity).orgId,
            parentId: (entity as DepartmentEntity).parentId,
            status: entity.status,
            sortOrder: entity.sortOrder,
          });
          invalidateDeptCache(); // 部门更新会影响部门树
          break;
        case 'post':
          await updatePost({
            id: entity.id,
            code: entity.code!,
            name: entity.name,
            status: entity.status,
            sortOrder: entity.sortOrder,
            remark: entity.remark,
            deptId: (entity as PostEntity).deptId,
            orgId: (entity as PostEntity).orgId,
          });
          break;
      }
    } catch (error) {
      console.error('更新失败:', error);
      throw error; // 重新抛出错误，让调用方处理
    } finally {
      setLoading(false);
    }
  }, [invalidateDeptCache]);

  // 删除实体 - 移除回调依赖，避免循环
  const deleteEntity = useCallback(async (entity: EntityType) => {
    console.log('deleteEntity 被调用:', entity);
    setLoading(true);
    try {
      switch (entity.type) {
        case 'org':
          console.log('删除组织:', entity.id);
          await deleteOrg({ id: entity.id });
          invalidateDeptCache(); // 组织删除会影响部门树
          break;
        case 'dept':
          console.log('删除部门:', entity.id);
          await deleteDept({ id: entity.id });
          invalidateDeptCache(); // 部门删除会影响部门树
          break;
        case 'post':
          console.log('删除岗位:', entity.id);
          await deletePost({ id: entity.id });
          break;
      }
      console.log('删除API调用成功');
    } catch (error) {
      console.error('删除失败:', error);
      throw error; // 重新抛出错误，让调用方处理
    } finally {
      setLoading(false);
    }
  }, [invalidateDeptCache]);

  // 移动实体（更改父级） - 移除回调依赖，避免循环
  const moveEntity = useCallback(async (entity: EntityType, newParentId: API.Int64) => {
    setLoading(true);
    try {
      switch (entity.type) {
        case 'org':
          await moveOrg({ id: entity.id, newParentId });
          invalidateDeptCache(); // 组织移动会影响部门树
          break;
        case 'dept':
          // 部门移动需要更新API支持，暂时使用更新接口
          await updateDept({
            id: entity.id,
            name: entity.name,
            code: entity.code,
            orgId: (entity as DepartmentEntity).orgId,
            parentId: newParentId,
            status: entity.status,
            sortOrder: entity.sortOrder,
          });
          invalidateDeptCache(); // 部门移动会影响部门树
          break;
        case 'post':
          // 岗位移动到新部门，需要更新API支持
          await updatePost({
            id: entity.id,
            code: entity.code!,
            name: entity.name,
            status: entity.status,
            sortOrder: entity.sortOrder,
            remark: entity.remark,
            deptId: newParentId,
            orgId: (entity as PostEntity).orgId,
          });
          break;
      }
    } catch (error) {
      console.error('移动失败:', error);
      throw error; // 重新抛出错误，让调用方处理
    } finally {
      setLoading(false);
    }
  }, [invalidateDeptCache]);

  // 排序更新（批量） - 移除回调依赖，避免循环
  const updateSortOrder = useCallback(async (entities: { id: API.Int64; sortOrder: number }[], type: EntityType['type']) => {
    setLoading(true);
    try {
      // 并发更新排序
      const updatePromises = entities.map(async (item) => {
        switch (type) {
          case 'org': {
            const orgResult = await updateOrg({
              id: item.id,
              sortOrder: item.sortOrder,
            });
            invalidateDeptCache(); // 组织排序会影响部门树
            return orgResult;
          }
          case 'dept': {
            // 需要获取完整的部门信息再更新
            const deptInfo = await getDeptById({ id: item.id });
            const deptResult = await updateDept({
              ...deptInfo.data,
              sortOrder: item.sortOrder,
            });
            invalidateDeptCache(); // 部门排序会影响部门树
            return deptResult;
          }
          case 'post': {
            // 岗位排序更新，需要API支持
            return updatePost({
              id: item.id,
              sortOrder: item.sortOrder,
              deptId: (item as PostEntity).deptId,
              orgId: (item as PostEntity).orgId,
            });
          }
        }
      });

      await Promise.all(updatePromises);
    } catch (error) {
      console.error('排序更新失败:', error);
      throw error; // 重新抛出错误，让调用方处理
    } finally {
      setLoading(false);
    }
  }, [invalidateDeptCache]);

  // 获取角色列表（用于岗位角色分配） - 移除回调依赖，避免循环
  const fetchRoles = useCallback(async (params: PageParams = {}) => {
    try {
      const response = await getRolePage({
        current: params.current || 1,
        size: params.size || 10,
        name: params.keyword,
        status: params.status,
      });

      return {
        data: response.data?.records || [],
        total: Number(response.data?.total) || 0,
      };
    } catch (error) {
      console.error('获取角色列表失败:', error);
      optionsRef.current.onError?.(error);
      return { data: [], total: 0 };
    }
  }, []); // 移除 onError 依赖

  // 扁平化返回所有部门（可按 orgId 过滤），用于 TreeSelect 选项
  const fetchAllDepartmentsFlat = useCallback(async (orgId?: API.Int64) => {
    // 确保有缓存
    if (!deptTreeCache.current) {
      const resp = await getDeptTree();
      deptTreeCache.current = resp.data || [];
    }
    const result: DepartmentEntity[] = [];
    const traverse = (items: API.SysDept[]) => {
      items.forEach((item) => {
        const itemOrgId = String(item.orgId);
        if (!orgId || itemOrgId === orgId) {
          result.push({
            id: String(item.id),
            name: item.name || '',
            code: item.code,
            status: item.status ?? 0,
            sortOrder: item.sortOrder ?? 0,
            createTime: item.createTime || '',
            updateTime: item.updateTime,
            remark: item.remark,
            orgId: itemOrgId,
            parentId: String(item.parentId),
            leaderUserId: item.leaderUserId,
            hierarchy: item.hierarchy,
            level: item.level,
            type: 'dept' as const,
          });
        }
        if (item.children && item.children.length) traverse(item.children);
      });
    };
    traverse(deptTreeCache.current);
    return result;
  }, []);

  return {
    loading,
    fetchOrganizations,
    fetchDepartments,
    fetchPosts,
    createEntity,
    updateEntity,
    deleteEntity,
    moveEntity,
    updateSortOrder,
    fetchRoles,
    fetchAllDepartmentsFlat,
  };
};

export type {
  EntityType,
  OrganizationEntity,
  DepartmentEntity,
  PostEntity,
  PageParams,
  DeptPageParams,
  PostPageParams,
};
