declare namespace API {
  /** Go int64/uint64 values use decimal strings on the business JSON wire. */
  type Int64 = string;

  /**
   * 通用分页结果
   */
  type PageResult<T> = {
    /** 当前页 */
    current?: number;
    /** 每页条数 */
    size?: number;
    /** 总条数 */
    total?: number;
    /** 数据列表 */
    list?: T[];
    /** 数据列表 (兼容 records 字段) */
    records?: T[];
  };

  type assignRoleMenusParams = {
    roleId: Int64;
  };

  type assignRolesToPostParams = {
    postId: Int64;
  };

  type getPostRolesParams = {
    postId: Int64;
  };

  type removeAllPostRolesParams = {
    postId: Int64;
  };

  type getPostsByRoleIdParams = {
    roleId: Int64;
  };

  type BatchLogoutResultVO = {
    successIds?: Int64[];
    failedIds?: Int64[];
    totalCount?: number;
    successCount?: number;
    failedCount?: number;
    failureReasons?: string[];
  };

  type bindMenuPermissionsParams = {
    menuId: Int64;
  };

  type changeStatusParams = {
    /** 组织ID */
    id: Int64;
    /** 状态（0正常 1停用） */
    status: number;
  };

  type checkCodeExistsParams = {
    /** 组织编码 */
    code: string;
    /** 排除的组织ID */
    excludeId?: Int64;
  };

  type checkFileExistParams = {
    /** Legacy Java upload scene; the Go endpoint ignores this value. */
    scene?: number;
    /** 文件 SHA256 */
    sha256?: string;
    /** 文件大小(byte) */
    size?: number;
    /** Go endpoint file size query name. */
    fileSize?: number;
  };

  type checkNameExistsParams = {
    /** 组织名称 */
    name: string;
    /** 排除的组织ID */
    excludeId?: Int64;
  };

  type checkPostCodeExistsParams = {
    /** 岗位编码 */
    code: string;
    /** 排除的岗位ID */
    excludeId?: Int64;
  };

  type checkPostNameExistsParams = {
    /** 岗位名称 */
    name: string;
    /** 排除的岗位ID */
    excludeId?: Int64;
  };

  type checkRoleCodeExistsParams = {
    /** 角色编码 */
    code: string;
    /** 排除的角色ID */
    excludeId?: Int64;
  };

  type checkRoleNameExistsParams = {
    /** 角色名称 */
    name: string;
    /** 排除的角色ID */
    excludeId?: Int64;
  };

  type checkUserAccountExistsParams = {
    /** 用户账号 */
    userAccount: string;
    /** 排除的用户ID */
    excludeId?: Int64;
  };

  type checkUserEmailExistsParams = {
    /** 用户邮箱 */
    userEmail: string;
    /** 排除的用户ID */
    excludeId?: Int64;
  };

  type checkUserPhoneExistsParams = {
    /** 用户手机号 */
    userPhone: string;
    /** 排除的用户ID */
    excludeId?: Int64;
  };

  type createCaptchaParams = {
    /** 验证码场景 */
    scene?: string;
    /** 验证码类型 */
    type?: string;
  };

  type deleteByIdsParams = {
    /** 主键ID列表 */
    ids: Int64[];
  };

  type deleteByIdParams = {
    /** 主键ID */
    id: Int64;
  };

  type deleteMenuByIdParams = {
    /** 菜单ID */
    id: Int64;
  };

  type deletePostByIdParams = {
    /** 岗位ID */
    id: Int64;
  };

  type deleteRoleByIdParams = {
    /** 角色ID */
    id: Int64;
  };

  type deleteUserByIdParams = {
    /** 用户ID */
    id: Int64;
  };

  type downloadFileParams = {
    /** 文件ID */
    fileId: Int64;
  };

  type getAdminInfoParams = {
    /** 管理员ID */
    id: Int64;
  };

  type getCaptchaParams = {
    /** 验证码ID */
    captchaId: string;
  };

  type getCurrentUserParams = {
    /** 用户ID */
    id: Int64;
  };

  type getDeptListParams = {
    /** 部门名称 */
    deptName?: string;
    /** 部门状态 */
    status?: number;
  };

  type getDeptTreeParams = {
    /** 部门名称 */
    deptName?: string;
    /** 部门状态 */
    status?: number;
  };

  type getFileInfoParams = {
    /** 文件ID */
    fileId: Int64;
  };

  type getLoginUserParams = {
    /** 用户ID */
    id: Int64;
  };

  type getMenuListParams = {
    /** 菜单名称 */
    menuName?: string;
    /** 菜单状态 */
    status?: number;
  };

  type getMenuTreeParams = {
    /** 菜单名称 */
    menuName?: string;
    /** 菜单状态 */
    status?: number;
  };

  type getOperationLogsParams = {
    /** 当前页 */
    current?: number;
    /** 每页条数 */
    size?: number;
    /** 操作用户名 */
    username?: string;
    /** 操作类型 */
    operationType?: string;
    /** 开始时间 */
    startTime?: string;
    /** 结束时间 */
    endTime?: string;
  };

  type getOrgListParams = {
    /** 组织名称 */
    orgName?: string;
    /** 组织状态 */
    status?: number;
  };

  type getOrgTreeParams = {
    /** 组织名称 */
    orgName?: string;
    /** 组织状态 */
    status?: number;
  };

  type getPostListParams = {
    /** 岗位名称 */
    postName?: string;
    /** 岗位编码 */
    postCode?: string;
    /** 岗位状态 */
    status?: number;
  };

  type getPostRolesParams = {
    /** 岗位ID */
    postId: Int64;
  };

  type getPostsByRoleIdParams = {
    /** 角色ID */
    roleId: Int64;
  };

  type getRoleListParams = {
    /** 角色名称 */
    roleName?: string;
    /** 角色状态 */
    status?: number;
  };

  type getRoleMenusParams = {
    /** 角色ID */
    roleId: Int64;
  };

  type getRolePermissionsParams = {
    /** 角色ID */
    roleId: Int64;
  };

  type getTemporaryPermissionsParams = {
    /** 用户ID */
    userId: Int64;
  };

  type getUserListParams = {
    /** 当前页 */
    current?: number;
    /** 每页条数 */
    size?: number;
    /** 用户账号 */
    userAccount?: string;
    /** 用户昵称 */
    userNickname?: string;
    /** 用户邮箱 */
    userEmail?: string;
    /** 用户手机号 */
    userPhone?: string;
    /** 用户状态 */
    status?: number;
    /** 部门ID */
    deptId?: Int64;
    /** 组织ID */
    orgId?: Int64;
    /** 岗位ID */
    postId?: Int64;
    /** 角色ID */
    roleId?: Int64;
  };

  type getUserPermissionsParams = {
    /** 用户ID */
    userId: Int64;
  };

  type getUserRolesParams = {
    /** 用户ID */
    userId: Int64;
  };

  type loginByWxOpenParams = {
    /** 微信授权码 */
    code: string;
    /** 微信开放平台应用ID */
    appId?: string;
  };

  type refreshTokenParams = Record<string, never>;

  type resetPasswordParams = {
    /** 用户ID */
    id: Int64;
  };

  type resetUserPasswordParams = {
    /** 用户ID */
    id: Int64;
  };

  type updateAdminStatusParams = {
    /** 管理员ID */
    id: Int64;
    /** 状态（0正常 1停用） */
    status: number;
  };

  type updateDeptStatusParams = {
    /** 部门ID */
    id: Int64;
    /** 状态（0正常 1停用） */
    status: number;
  };

  type updateMenuStatusParams = {
    /** 菜单ID */
    id: Int64;
    /** 状态（0正常 1停用） */
    status: number;
  };

  type updateOrgStatusParams = {
    /** 组织ID */
    id: Int64;
    /** 状态（0正常 1停用） */
    status: number;
  };

  type updatePostStatusParams = {
    /** 岗位ID */
    id: Int64;
    /** 状态（0正常 1停用） */
    status: number;
  };

  type updateRoleStatusParams = {
    /** 角色ID */
    id: Int64;
    /** 状态（0正常 1停用） */
    status: number;
  };

  type updateUserStatusParams = {
    /** 用户ID */
    id: Int64;
    /** 状态（0正常 1停用） */
    status: number;
  };

  type uploadFileParams = {
    /** 上传业务场景 */
    scene?: string;
  };

  type userLoginByWxOpenParams = {
    /** 微信授权码 */
    code: string;
    /** 微信开放平台应用ID */
    appId?: string;
  };

  type verifyCaptchaParams = {
    /** 验证码ID */
    captchaId: string;
    /** 验证码答案 */
    captchaAnswer: string;
  };

  type AdminCreateRequest = {
    /** 管理员账号 */
    adminAccount: string;
    /** 管理员昵称 */
    adminNickname: string;
    /** 管理员邮箱 */
    adminEmail?: string;
    /** 管理员手机号 */
    adminPhone?: string;
    /** 管理员密码 */
    adminPassword: string;
    /** 管理员头像 */
    adminAvatar?: string;
    /** 管理员状态（0正常 1停用） */
    status?: number;
    /** 管理员备注 */
    remark?: string;
  };

  type AdminUpdateRequest = {
    /** 管理员ID */
    id: Int64;
    /** 管理员账号 */
    adminAccount: string;
    /** 管理员昵称 */
    adminNickname: string;
    /** 管理员邮箱 */
    adminEmail?: string;
    /** 管理员手机号 */
    adminPhone?: string;
    /** 管理员头像 */
    adminAvatar?: string;
    /** 管理员状态（0正常 1停用） */
    status?: number;
    /** 管理员备注 */
    remark?: string;
  };

  type AdminVO = {
    /** 管理员ID */
    id?: Int64;
    /** 管理员账号 */
    adminAccount?: string;
    /** 管理员昵称 */
    adminNickname?: string;
    /** 管理员邮箱 */
    adminEmail?: string;
    /** 管理员手机号 */
    adminPhone?: string;
    /** 管理员头像 */
    adminAvatar?: string;
    /** 管理员状态（0正常 1停用） */
    status?: number;
    /** 管理员备注 */
    remark?: string;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
  };

  type BatchLogoutRequest = {
    /** 用户ID列表 */
    userIds: Int64[];
  };

  type CaptchaCreateRequest = {
    /** 验证码场景 */
    scene?: string;
    /** 验证码类型 */
    type?: string;
  };

  type CaptchaVO = {
    /** 验证码图片（Base64） */
    codeImage?: string;
    /** 验证码ID */
    uuid?: string;
    /** 新挑战协议中的验证码ID */
    captchaId?: string;
    /** 新挑战协议中的验证码图片（Base64） */
    captchaImage?: string;
  };

  type DeptCreateRequest = {
    /** 部门名称 */
    name: string;
    /** 部门编码 */
    code: string;
    /** 所属组织ID */
    orgId?: Int64;
    /** 父部门ID */
    parentId?: Int64;
    /** 负责人用户ID */
    leaderUserId?: Int64;
    /** 部门状态（0正常 1停用） */
    status?: number;
    /** 显示顺序 */
    sortOrder?: number;
  };

  type DeptTreeVO = {
    /** 部门ID */
    id?: Int64;
    /** 部门名称 */
    name?: string;
    /** 部门编码 */
    code?: string;
    /** 所属组织ID */
    orgId?: Int64;
    /** 父部门ID */
    parentId?: Int64;
    /** 负责人用户ID */
    leaderUserId?: Int64;
    /** 部门状态（0正常 1停用） */
    status?: number;
    /** 显示顺序 */
    sortOrder?: number;
    /** 层级路径 */
    hierarchy?: string;
    /** 层级级别 */
    level?: number;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
    /** 子部门列表 */
    children?: DeptTreeVO[];
  };

  type DeptUpdateRequest = {
    /** 部门ID */
    id: Int64;
    /** 部门名称 */
    name: string;
    /** 部门编码 */
    code: string;
    /** 所属组织ID */
    orgId?: Int64;
    /** 父部门ID */
    parentId?: Int64;
    /** 负责人用户ID */
    leaderUserId?: Int64;
    /** 部门状态（0正常 1停用） */
    status?: number;
    /** 显示顺序 */
    sortOrder?: number;
  };

  type DeptVO = {
    /** 部门ID */
    id?: Int64;
    /** 部门名称 */
    name?: string;
    /** 部门编码 */
    code?: string;
    /** 所属组织ID */
    orgId?: Int64;
    /** 父部门ID */
    parentId?: Int64;
    /** 负责人用户ID */
    leaderUserId?: Int64;
    /** 部门状态（0正常 1停用） */
    status?: number;
    /** 显示顺序 */
    sortOrder?: number;
    /** 层级路径 */
    hierarchy?: string;
    /** 层级级别 */
    level?: number;
    /** 子部门列表 */
    children?: DeptVO[];
    /** 备注 */
    remark?: string;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
  };

  type FileInfoVO = {
    /** 文件ID */
    fileId?: Int64;
    /** 文件名 */
    fileName?: string;
    /** 文件大小 */
    fileSize?: number;
    /** 文件类型 */
    fileType?: string;
    /** 文件路径 */
    filePath?: string;
    /** 文件URL */
    fileUrl?: string;
    /** 上传业务场景 */
    scene?: string;
    /** 创建时间 */
    createTime?: string;
  };

  type LoginUserVO = {
    /** 用户ID */
    id?: Int64;
    /** 用户名 */
    username?: string;
    /** 用户昵称 */
    nickname?: string;
    /** 用户头像 */
    userAvatar?: string;
    /** 用户角色名称列表 */
    userRole?: string[];
    /** 用户岗位名称列表 */
    userPosition?: string[];
    /** 用户组织名称列表 */
    organizations?: string[];
    /** 用户部门名称列表 */
    departments?: string[];
    /** 用户权限代码列表 */
    permissions?: string[];
    /** 角色编码列表 */
    roleCodes?: string[];
    /** 岗位编码列表 */
    postCodes?: string[];
    /** 组织编码列表 */
    orgCodes?: string[];
    /** 部门编码列表 */
    deptCodes?: string[];
    /** 后端判定的全局管理员标识 */
    isAdmin?: boolean;
    /** 当前主组织，参与配置与字典缓存隔离 */
    primaryOrgId?: Int64;
    /** 后端授权版本，角色或权限变化时递增 */
    authVersion?: Int64;
    /** 后端权威数据范围 */
    dataScope?: UserDataScopeVO;
    /** 登录时间 */
    loginTime?: string;
  };

  type CurrentUserResponse = {
    /** 用户ID */
    id?: Int64;
    /** 用户名 */
    username?: string;
    /** 用户昵称 */
    nickname?: string;
    /** 用户头像 */
    userAvatar?: string;
    /** 用户角色名称列表 */
    userRole?: string[];
    /** 用户岗位名称列表 */
    userPosition?: string[];
    /** 用户组织名称列表 */
    organizations?: string[];
    /** 用户部门名称列表 */
    departments?: string[];
    /** 用户权限代码列表 */
    permissions?: string[];
    /** 角色编码列表 */
    roleCodes?: string[];
    /** 岗位编码列表 */
    postCodes?: string[];
    /** 组织编码列表 */
    orgCodes?: string[];
    /** 部门编码列表 */
    deptCodes?: string[];
    /** 后端判定的全局管理员标识 */
    isAdmin?: boolean;
    /** 当前主组织，参与配置与字典缓存隔离 */
    primaryOrgId?: Int64;
    /** 后端授权版本，角色或权限变化时递增 */
    authVersion?: Int64;
    /** 后端权威数据范围 */
    dataScope?: UserDataScopeVO;
  };

  type UserDataScopeVO = {
    userId?: Int64;
    deptIds?: Int64[];
    orgIds?: Int64[];
    scopeType?: 'ALL' | 'CUSTOM' | 'DEPT' | 'DEPT_AND_CHILD' | 'SELF' | 'NONE';
  };

  type LoginResponse = {
    /** 当前登录用户 */
    user?: CurrentUserResponse;
    /** 访问令牌 */
    accessToken?: string;
    /** 令牌类型 */
    tokenType?: string;
    /** 访问令牌过期时间（秒） */
    accessTtlSec?: number;
    /** 是否首次登录 */
    firstLogin?: boolean;
  };

  type MenuCreateRequest = {
    /** 菜单名称 */
    name: string;
    /** 父菜单ID */
    parentId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 路由地址 */
    path?: string;
    /** 组件路径 */
    component?: string;
    /** 菜单类型（M目录 C菜单 F按钮） */
    type?: string;
    /** 菜单图标 */
    icon?: string;
    /** 菜单状态（0正常 1停用） */
    status?: number;
    /** 权限标识 */
    permission?: string;
    /** 是否外链 */
    isFrame?: number;
    /** 是否缓存 */
    isCache?: number;
    /** 是否显示 */
    visible?: number;
    /** 备注 */
    remark?: string;
  };

  type MenuTreeVO = {
    /** 菜单ID */
    id?: Int64;
    /** 菜单名称 */
    name?: string;
    /** 父菜单ID */
    parentId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 路由地址 */
    path?: string;
    /** 组件路径 */
    component?: string;
    /** 菜单类型（M目录 C菜单 F按钮） */
    type?: string;
    /** 菜单图标 */
    icon?: string;
    /** 菜单状态（0正常 1停用） */
    status?: number;
    /** 权限标识 */
    permission?: string;
    /** 运行时能力编码 */
    featureCode?: string;
    /** 是否外链 */
    isFrame?: number;
    /** 是否缓存 */
    isCache?: number;
    /** 是否显示 */
    visible?: number;
    /** 备注 */
    remark?: string;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
    /** 子菜单列表 */
    children?: MenuTreeVO[];
  };

  type MenuUpdateRequest = {
    /** 菜单ID */
    id: Int64;
    /** 菜单名称 */
    name: string;
    /** 父菜单ID */
    parentId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 路由地址 */
    path?: string;
    /** 组件路径 */
    component?: string;
    /** 菜单类型（M目录 C菜单 F按钮） */
    type?: string;
    /** 菜单图标 */
    icon?: string;
    /** 菜单状态（0正常 1停用） */
    status?: number;
    /** 权限标识 */
    permission?: string;
    /** 是否外链 */
    isFrame?: number;
    /** 是否缓存 */
    isCache?: number;
    /** 是否显示 */
    visible?: number;
    /** 备注 */
    remark?: string;
  };

  type MenuVO = {
    /** 菜单ID */
    id?: Int64;
    /** 菜单名称 */
    name?: string;
    /** 父菜单ID */
    parentId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 路由地址 */
    path?: string;
    /** 组件路径 */
    component?: string;
    /** 菜单类型（M目录 C菜单 F按钮） */
    type?: string;
    /** 菜单状态（0正常 1停用） */
    status?: number;
    /** 权限标识 */
    permission?: string;
    /** 运行时能力编码 */
    featureCode?: string;
    /** 菜单图标 */
    icon?: string;
    /** 是否外链 */
    isFrame?: number;
    /** 是否缓存 */
    isCache?: number;
    /** 是否显示 */
    visible?: number;
    /** 备注 */
    remark?: string;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
    /** 子菜单列表 */
    children?: MenuVO[];
  };

  type OrgCreateRequest = {
    /** 组织名称 */
    name: string;
    /** 组织编码 */
    code: string;
    /** 父组织ID */
    parentId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 负责人用户ID */
    leaderUserId?: Int64;
    /** 组织状态（0正常 1停用） */
    status?: number;
  };

  type OrgTreeVO = {
    /** 组织ID */
    id?: Int64;
    /** 组织名称 */
    name?: string;
    /** 组织编码 */
    code?: string;
    /** 父组织ID */
    parentId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 负责人用户ID */
    leaderUserId?: Int64;
    /** 组织状态（0正常 1停用） */
    status?: number;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
    /** 父组织名称 */
    parentName?: string;
    /** 子组织列表 */
    children?: OrgTreeVO[];
  };

  type OrgUpdateRequest = {
    /** 组织ID */
    id: Int64;
    /** 组织名称 */
    name: string;
    /** 组织编码 */
    code: string;
    /** 父组织ID */
    parentId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 负责人用户ID */
    leaderUserId?: Int64;
    /** 组织状态（0正常 1停用） */
    status?: number;
  };

  type OrgVO = {
    /** 组织ID */
    id?: Int64;
    /** 组织名称 */
    name?: string;
    /** 组织编码 */
    code?: string;
    /** 父组织ID */
    parentId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 负责人用户ID */
    leaderUserId?: Int64;
    /** 组织状态（0正常 1停用） */
    status?: number;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
    /** 父组织名称 */
    parentName?: string;
    /** 子组织列表 */
    children?: OrgVO[];
  };

  type PageSysOperationLog = {
    /** 当前页 */
    current?: number;
    /** 每页条数 */
    size?: number;
    /** 总条数 */
    total?: number;
    /** 数据列表 */
    records?: SysOperationLog[];
  };

  type PostCreateRequest = {
    /** 岗位名称 */
    name: string;
    /** 岗位编码 */
    code: string;
    /** 所属部门ID */
    deptId?: Int64;
    /** 所属组织ID */
    orgId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 岗位状态（0正常 1停用） */
    status?: number;
    /** 备注 */
    remark?: string;
  };

  type PostUpdateRequest = {
    /** 岗位ID */
    id: Int64;
    /** 岗位名称 */
    name: string;
    /** 岗位编码 */
    code: string;
    /** 所属部门ID */
    deptId?: Int64;
    /** 所属组织ID */
    orgId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 岗位状态（0正常 1停用） */
    status?: number;
    /** 备注 */
    remark?: string;
  };

  type PostVO = {
    /** 岗位ID */
    id?: Int64;
    /** 岗位名称 */
    name?: string;
    /** 岗位编码 */
    code?: string;
    /** 所属部门ID */
    deptId?: Int64;
    /** 所属组织ID */
    orgId?: Int64;
    /** 显示顺序 */
    sortOrder?: number;
    /** 岗位状态（0正常 1停用） */
    status?: number;
    /** 备注 */
    remark?: string;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
  };

  type RefreshTokenRequest = Record<string, never>;

  type RefreshTokenResponse = {
    /** 访问令牌 */
    accessToken?: string;
    /** 令牌类型 */
    tokenType?: string;
    /** 访问令牌过期时间（秒） */
    accessTtlSec?: number;
  };

  type RefreshResponse = {
    /** 访问令牌 */
    accessToken?: string;
    /** 令牌类型 */
    tokenType?: string;
    /** 访问令牌过期时间（秒） */
    accessTtlSec?: number;
  };

  type ResultAdminVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: AdminVO;
  };

  type ResultBatchLogoutResultVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: BatchLogoutResultVO;
  };

  type ResultBoolean = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: boolean;
  };

  type ResultCurrentUserResponse = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: CurrentUserResponse;
  };

  type ResultCaptchaVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: CaptchaVO;
  };

  type ResultDeptTreeVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: DeptTreeVO[];
  };

  type ResultDeptVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: DeptVO;
  };

  type ResultFileInfoVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: FileInfoVO;
  };

  type ResultListDeptVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: DeptVO[];
  };

  type ResultListMenuVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: MenuVO[];
  };

  type ResultListOrgVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: OrgVO[];
  };

  type ResultListPostVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: PostVO[];
  };

  type ResultListRoleVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: RoleVO[];
  };

  type ResultListString = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: string[];
  };

  type ResultListTemporaryPermissionVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: TemporaryPermissionVO[];
  };

  type ResultListUserVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: UserVO[];
  };

  type ResultLoginResponse = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: LoginResponse;
  };

  type ResultLoginUserVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: LoginUserVO;
  };

  type ResultMenuTreeVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: MenuTreeVO[];
  };

  type ResultMenuVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: MenuVO;
  };

  type ResultOrgTreeVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: OrgTreeVO[];
  };

  type ResultOrgVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: OrgVO;
  };

  type ResultPageSysOperationLog = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: PageSysOperationLog;
  };

  type ResultPostVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: PostVO;
  };

  type ResultRefreshTokenResponse = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: RefreshTokenResponse;
  };

  type ResultRefreshResponse = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: RefreshResponse;
  };

  type ResultRoleVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: RoleVO;
  };

  type ResultString = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: string;
  };

  type ResultUserVO = {
    /** 状态码 */
    code?: number;
    /** 返回消息 */
    message?: string;
    /** 返回数据 */
    data?: UserVO;
  };

  type ResultUserDetailVO = Result<UserVO>;

  type RoleCreateRequest = {
    /** 角色名称 */
    name: string;
    /** 角色编码 */
    code: string;
    /** 显示顺序 */
    sortOrder?: number;
    /** 角色状态（0正常 1停用） */
    status?: number;
    /** 数据权限范围 */
    dataScope?: number;
    /** 角色类型 */
    type?: string;
    /** 备注 */
    remark?: string;
  };

  type RoleUpdateRequest = {
    /** 角色ID */
    id: Int64;
    /** 角色名称 */
    name: string;
    /** 角色编码 */
    code: string;
    /** 显示顺序 */
    sortOrder?: number;
    /** 角色状态（0正常 1停用） */
    status?: number;
    /** 数据权限范围 */
    dataScope?: number;
    /** 角色类型 */
    type?: string;
    /** 备注 */
    remark?: string;
  };

  type RoleVO = {
    /** 角色ID */
    id?: Int64;
    /** 角色名称 */
    name?: string;
    /** 角色编码 */
    code?: string;
    /** 角色状态（0正常 1停用） */
    status?: number;
    /** 数据权限范围 */
    dataScope?: number;
    /** 角色类型 */
    type?: string;
    /** 显示顺序 */
    sortOrder?: number;
    /** 备注 */
    remark?: string;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
    /** 是否为系统管理角色 */
    systemManaged?: boolean;
    /** 是否为全局授权安全根 */
    authorizationRoot?: boolean;
    /** 角色授权快照修订号 */
    grantRevision?: Int64;
  };

  type RoleSecurityStatusVO = {
    rootRoleId?: Int64;
    rootRoleCode?: string;
    activeDirectAdmins?: number;
    minimumRequired?: number;
    recommendedMinimum?: number;
    health?: 'HEALTHY' | 'LOW_REDUNDANCY';
    warnings?: string[];
  };

  type ResultRoleSecurityStatusVO = Result<RoleSecurityStatusVO>;

  type SysOperationLog = {
    /** ID */
    id?: Int64;
    /** 操作用户ID */
    userId?: Int64;
    /** 操作用户名 */
    userName?: string;
    /** 用户昵称 */
    nickName?: string;
    /** 操作类型 */
    operationType?: string;
    /** 操作类型描述 */
    operationTypeDesc?: string;
    /** 面向用户展示的操作名称 */
    operationTypeLabel?: string;
    /** 操作描述 */
    operationDesc?: string;
    /** 方法名 */
    methodName?: string;
    /** 请求方法 */
    requestMethod?: string;
    /** 请求URL */
    requestUrl?: string;
    /** 请求IP */
    requestIp?: string;
    /** 请求地理位置 */
    requestLocation?: string;
    /** 浏览器信息 */
    browser?: string;
    /** 操作系统 */
    os?: string;
    /** 操作时间 */
    operationTime?: string;
    /** 执行时间（毫秒） */
    executionTime?: number;
    /** 操作状态：0失败 1成功 */
    status?: number;
    /** 错误消息 */
    errorMsg?: string;
    /** 创建时间 */
    createTime?: string;
  };

  type TemporaryPermissionCreateRequest = {
    /** 用户ID */
    userId: Int64;
    /** 权限标识列表 */
    permissions: string[];
    /** 过期时间 */
    expireTime: string;
    /** 备注 */
    remark?: string;
  };

  type TemporaryPermissionUpdateRequest = {
    /** 临时权限ID */
    id: Int64;
    /** 用户ID */
    userId: Int64;
    /** 权限标识列表 */
    permissions: string[];
    /** 过期时间 */
    expireTime: string;
    /** 备注 */
    remark?: string;
  };

  type TemporaryPermissionVO = {
    /** 临时权限ID */
    id?: Int64;
    /** 用户ID */
    userId?: Int64;
    /** 权限标识列表 */
    permissions?: string[];
    /** 过期时间 */
    expireTime?: string;
    /** 备注 */
    remark?: string;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
  };

  type UserCreateRequest = {
    /** 用户名 */
    username: string;
    /** 用户昵称 */
    nickname: string;
    /** 用户邮箱 */
    email?: string;
    /** 用户手机号 */
    userPhone?: string;
    /** 用户密码 */
    password: string;
    /** 用户性别（取值来自 sys_dict 的 gender 字典） */
    userGender?: number;
    /** 用户状态（0正常 1停用） */
    status?: number;
    /** 用户备注 */
    remark?: string;
  };

  type UserRoleAssignRequest = {
    userId: Int64;
    roleIds: Int64[];
  };

  type UserOrgAssignRequest = {
    userId: Int64;
    orgIds: Int64[];
    primaryOrgId?: Int64;
  };

  type UserLoginRequest = {
    /** 用户账号 */
    userAccount: string;
    /** 用户密码 */
    userPassword: string;
    /** 设备ID */
    deviceId?: string;
    /** 验证码 */
    captcha?: string;
    /** 验证码ID */
    captchaId?: string;
  };

  type UserUpdateRequest = {
    /** 用户ID */
    id: Int64;
    /** 用户名 */
    username?: string;
    /** 用户昵称 */
    nickname?: string;
    /** 用户邮箱 */
    email?: string;
    /** 用户手机号 */
    userPhone?: string;
    /** 用户性别（取值来自 sys_dict 的 gender 字典） */
    userGender?: number;
    /** 用户状态（0正常 1停用） */
    status?: number;
    /** 用户备注 */
    remark?: string;
  };

  type UserVO = {
    /** 用户ID */
    id?: Int64;
    /** 用户名 */
    username?: string;
    /** 用户昵称 */
    nickname?: string;
    /** 用户头像 */
    avatar?: string;
    /** 用户邮箱 */
    email?: string;
    /** 用户手机号 */
    userPhone?: string;
    /** 用户性别（取值来自 sys_dict 的 gender 字典） */
    userGender?: number;
    /** 用户状态（0正常 1停用） */
    status?: number;
    /** 创建时间 */
    createTime?: string;
    /** 更新时间 */
    updateTime?: string;
    /** 扁平化角色名称 */
    allRoleNames?: string[];
    /** 直接角色名称回退信息 */
    fallbackRoleNames?: string[];
    /** 扁平化角色编码 */
    allRoleCodes?: string[];
    /** 扁平化组织名称 */
    allOrgNames?: string[];
    /** 直接组织名称回退信息 */
    fallbackOrgNames?: string[];
    /** 扁平化组织编码 */
    allOrgCodes?: string[];
    /** 扁平化部门名称 */
    allDeptNames?: string[];
    /** 扁平化部门编码 */
    allDeptCodes?: string[];
    /** 扁平化岗位名称 */
    allPostNames?: string[];
    /** 扁平化岗位编码 */
    allPostCodes?: string[];
    /** 扁平化权限列表 */
    allPermissions?: string[];
  };

  type WxMpLoginRequest = {
    /** 微信授权码 */
    code: string;
    /** 微信开放平台应用ID */
    appId?: string;
  };

  // ============ File Service Types ============

  /** 文件信息 (兼容别名) */
  type FileInfo = {
    id?: Int64;
    fileName?: string;
    fileInnerName?: string;
    fileSize?: number;
    contentType?: string;
    fileSha256?: string;
    fileCrc32c?: string;
    hashAlgorithm?: string;
    integrityStatus?: string;
    integrityCheckedAt?: string;
    securityVerdict?: string;
    distributionMode?: string;
    fileUrl?: string;
    bizType?: number;
    storagePath?: string;
    status?: string;
    scanStatus?: string;
    referenceCount?: number;
    createTime?: string;
    updateTime?: string;
  };

  /** 存储策略 */
  type StorageStrategy = {
    id?: Int64;
    strategyName?: string;
    providerType?: string;
    configJson?: string;
    isEnabled?: boolean;
    isDefault?: boolean;
    runState?: 'ACTIVE' | 'DRAINING' | 'DISABLED' | string;
    priority?: number;
    healthStatus?: number;
    lastHealthCheck?: string;
    failureRateThreshold?: number;
    createTime?: string;
    updateTime?: string;
  };

  /** 文件处理任务 */
  type FileProcessTask = {
    id?: Int64;
    fileId?: Int64;
    taskType?: string;
    pipelineId?: string;
    nodeId?: string;
    idempotencyKey?: string;
    dedupKey?: string;
    replayToken?: string;
    dependsOn?: string;
    attempt?: number;
    status?: number;
    retryCount?: number;
    maxRetry?: number;
    taskParams?: string;
    resultData?: string;
    errorMsg?: string;
    createTime?: string;
    updateTime?: string;
    startTime?: string;
    finishTime?: string;
  };

  /** 分块上传初始化响应 */
  type ChunkUploadInitResponse = {
    uploadId: string;
    chunkSize: number;
    totalChunks: number;
    expireAt: string;
    storageMode: string;
    uploadedChunks: number[];
  };

  /** 分块上传状态响应 */
  type ChunkUploadStatusResponse = {
    uploadId: string;
    status: number;
    statusName: string;
    fileName: string;
    fileSize: number;
    chunkSize: number;
    totalChunks: number;
    uploadedChunks: number[];
    expireAt?: string;
  };

  /** 文件统计 */
  type FileStats = {
    totalCount: number;
    totalSize: number;
    totalSizeFormatted: string;
    imageCount: number;
    docCount: number;
    videoCount: number;
  };

  /** 任务统计 */
  type TaskStats = {
    pending: number;
    processing: number;
    completed: number;
    failed: number;
    pendingRetry?: number;
  };

  type TaskStatsResponse = TaskStats;

  /** 健康检查响应 */
  type HealthCheckResponse = {
    healthy: boolean;
    message?: string;
  };

  // Result Types
  type ResultPageFileInfo = Result<PageResult<FileInfo>>;
  type ResultFileInfo = Result<FileInfo>;
  type ResultFileReferenceList = Result<FileReference[]>;
  type ResultFileStats = Result<FileStats>;
  type ResultPageStorageStrategy = Result<PageResult<StorageStrategy>>;
  type ResultStorageStrategy = Result<StorageStrategy>;
  type ResultInt64 = Result<Int64>;
  type ResultHealthCheckResponse = Result<HealthCheckResponse>;
  type ResultStringList = Result<string[]>;
  type ResultCheckFileExistResponse = Result<CheckFileExistResponse>;
  type ResultChunkUploadInitResponse = Result<ChunkUploadInitResponse>;
  type ResultChunkUploadStatusResponse = Result<ChunkUploadStatusResponse>;
  type ResultChunkUploadStatusList = Result<ChunkUploadStatusResponse[]>;
  type ResultPageFileProcessTask = Result<PageResult<FileProcessTask>>;
  type ResultFileProcessTask = Result<FileProcessTask>;
  type ResultTaskStatsResponse = Result<TaskStats>;
  type ResultBoolean = Result<boolean>;

  /** 文件秒传挑战信息 */
  type FileHmacInfo = {
    nonce: string;
    challenge: string;
    timestamp: number;
    offset: number;
    length: number;
  };

  /** 秒传检查响应 */
  type CheckFileExistResponse = {
    exists: boolean;
    fileId?: Int64;
  };

  /** Upload-only request accepted by the faster-upload endpoint. */
  type UploadFileRequest = {
    fileName: string;
    contentType?: string;
    sha256?: string;
    fileSize?: number;
  };

  type UploadResult = {
    fileId: Int64;
  };

  type ChunkPartResponse = {
    uploadId: string;
    partNumber: number;
    sha256: string;
    uploaded: boolean;
    uploadedChunks: number[];
  };

  type ResultUploadResult = Result<UploadResult>;
  type ResultUploadTaskStatus = Result<UploadTaskStatus>;
  type ResultChunkPartResponse = Result<ChunkPartResponse>;

  type UploadTaskStatus = {
    taskId: string;
    status: string;
    expireAt?: string;
    fileId?: Int64;
  };

  /** 文件引用 */
  type FileReference = {
    id: Int64;
    fileId: Int64;
    userId: Int64;
    displayName?: string;
    visitUrl?: string;
    bizType?: string;
    bizId?: Int64;
    accessLevel?: number;
    accessScope?: 'OWNER_ONLY' | 'DELEGATED' | 'LOGIN_USERS' | 'PUBLIC' | string;
    visitStrategy?: string;
    createTime: string;
    updateTime?: string;
  };

  /** 通用结果类型 */
  type Result<T> = {
    code: number;
    data?: T;
    message?: string;
    traceId?: string;
  };

  type downloadFileByIdParams = {
    fileId?: Int64;
    referenceId?: Int64;
    token?: string;
  };

  // ====== RBAC / System Types (aligned to backend) ======

  type SysOrg = OrgVO;

  type SysDept = DeptVO;

  type SysPost = PostVO;

  type RoleCommandDTO = {
    id?: Int64;
    name?: string;
    code?: string;
    status?: number;
    sort?: number;
    dataScope?: number;
    type?: string;
    sortOrder?: number;
    remark?: string;
  };

  type MenuCommandDTO = {
    id?: Int64;
    parentId?: Int64;
    name?: string;
    sortOrder?: number;
    path?: string;
    component?: string;
    type?: string;
    status?: number;
    permission?: string;
    icon?: string;
    isFrame?: number;
    isCache?: number;
    visible?: number;
    remark?: string;
  };

  type PermissionCommandDTO = {
    id?: Int64;
    code?: string;
    name?: string;
    resourceType?: string;
    method?: string;
    path?: string;
    status?: number;
    description?: string;
  };

  type MenuPermissionAssignDTO = {
    menuId?: Int64;
    permissionIds?: Int64[];
  };

  type PermissionVO = {
    id?: Int64;
    code?: string;
    name?: string;
    resourceType?: string;
    method?: string;
    path?: string;
    status?: number;
    description?: string;
    createTime?: string;
    updateTime?: string;
  };

  type OperationLogVO = SysOperationLog;

  type OnlineUserVO = {
    userId?: Int64;
    username?: string;
    nickname?: string;
    avatar?: string;
    email?: string;
    userRole?: string;
    loginTime?: number;
    lastActiveTime?: number;
    expireTime?: number;
    loginIp?: string;
    loginAddress?: string;
    browser?: string;
    os?: string;
    deviceId?: string;
    userAgent?: string;
    tokenId?: string;
    isCurrentSession?: boolean;
  };

  type OnlineUserStatsVO = {
    totalOnlineUsers?: number;
    adminUsers?: number;
    normalUsers?: number;
    browserStats?: Record<string, number>;
    osStats?: Record<string, number>;
    todayLoginUsers?: number;
    activeUsers?: number;
    totalOnline?: number;
    todayLogin?: number;
    peakOnline?: number;
  };

  type UserSessionVO = {
    sessionId?: string;
    userId?: Int64;
    username?: string;
    nickname?: string;
    loginTime?: string;
    lastActiveTime?: string;
    ipAddress?: string;
    deviceId?: string;
    deviceInfo?: string;
    userAgent?: string;
    currentSession?: boolean;
    revoked?: boolean;
    refreshFamilyId?: string;
  };

  type UserDeviceVO = {
    deviceId?: string;
    deviceInfo?: string;
    sessionCount?: number;
    lastActiveTime?: string;
    currentDevice?: boolean;
    ipSamples?: string[];
  };

  type StepUpChallengeRequest = {
    scene?: string;
  };

  type StepUpChallengeVO = {
    challengeId?: string;
    scene?: string;
    expiresInSeconds?: number;
  };

  type StepUpVerifyRequest = {
    challengeId?: string;
    password?: string;
  };

  type StepUpTokenVO = {
    stepUpToken?: string;
    scene?: string;
    expiresInSeconds?: number;
  };

  type IPage<T> = {
    records?: T[];
    total?: number;
    size?: number;
    current?: number;
    pages?: number;
  };

  type ResultSysOrg = Result<SysOrg>;
  type ResultListSysOrg = Result<SysOrg[]>;
  type ResultSysDept = Result<SysDept>;
  type ResultListSysDept = Result<SysDept[]>;
  type ResultSysPost = Result<SysPost>;
  type ResultListSysPost = Result<SysPost[]>;
  type ResultIPageSysPost = Result<IPage<SysPost>>;
  type ResultIPageRoleVO = Result<IPage<RoleVO>>;
  type ResultPermissionVO = Result<PermissionVO>;
  type ResultListPermissionVO = Result<PermissionVO[]>;
  type ResultListLong = Result<Int64[]>;
  type ResultSysOperationLog = Result<SysOperationLog>;
  type ResultPageOnlineUserVO = Result<IPage<OnlineUserVO>>;
  type ResultOnlineUserVO = Result<OnlineUserVO>;
  type ResultOnlineUserStatsVO = Result<OnlineUserStatsVO>;
  type ResultIPageOperationLogVO = Result<IPage<OperationLogVO>>;
  type ResultListUserSessionVO = Result<UserSessionVO[]>;
  type ResultListUserDeviceVO = Result<UserDeviceVO[]>;
  type ResultStepUpChallengeVO = Result<StepUpChallengeVO>;
  type ResultStepUpTokenVO = Result<StepUpTokenVO>;
  type ResultRecordStringLong = Result<Record<string, number>>;
  type ResultInteger = Result<number>;
  type OperationTypeOption = {
    value: string;
    label: string;
  };
  type ResultOperationTypeOptionArray = Result<OperationTypeOption[]>;

  type getRolePageParams = {
    current?: number;
    size?: number;
    name?: string;
    status?: number;
  };

  type getRoleByIdParams = {
    id: Int64;
  };

  type deleteRoleParams = {
    id: Int64;
  };

  type getRoleMenuIdsParams = {
    roleId: Int64;
  };

  type getUserRoleIdsParams = {
    id: Int64;
  };

  type getUserOrgIdsParams = {
    id: Int64;
  };

  type getDataScopeUserListParams = {
    current?: number;
    size?: number;
    pageSize?: number;
    username?: string;
    nickname?: string;
    status?: number;
    orgId?: Int64;
    deptId?: Int64;
    postId?: Int64;
  };

  type deleteUserParams = {
    id: Int64;
  };

  type getUserDetailParams = {
    id: Int64;
  };

  type getUserByIdParams = {
    id: Int64;
  };

  type listUsersParams = {
    current?: number;
    size?: number;
    pageSize?: number;
    username?: string;
    nickname?: string;
    status?: number;
    orgId?: Int64;
    deptId?: Int64;
    postId?: Int64;
    sortField?: string;
    sortOrder?: string;
  };

  type ResultPageUserVO = Result<IPage<UserVO>>;

  type SimpleUserVO = {
    id?: Int64;
    username?: string;
    nickName?: string;
    avatar?: string;
    status?: number;
  };

  type UserOptionVO = SimpleUserVO;
  type ResultListUserOptionVO = Result<UserOptionVO[]>;
  type ResultListSimpleUserVO = Result<SimpleUserVO[]>;
  type ResultSimpleUserVO = Result<SimpleUserVO>;

  type getMenuByIdParams = {
    id: Int64;
  };

  type getUserMenusParams = {
    userId: Int64;
  };

  type deleteMenuParams = {
    id: Int64;
  };

  type getMenuPermissionIdsParams = {
    menuId: Int64;
  };

  type listPermissionsParams = {
    current?: number;
    pageSize?: number;
    size?: number;
    code?: string;
    name?: string;
    resourceType?: string;
    method?: string;
    path?: string;
    status?: number;
  };

  type getPermissionParams = {
    permissionId: Int64;
  };

  type updatePermissionParams = {
    permissionId: Int64;
  };

  type deletePermissionParams = {
    permissionId: Int64;
  };

  type getOrgByIdParams = {
    id: Int64;
  };

  type getOrgByCodeParams = {
    code: string;
  };

  type getOrgByUserIdParams = {
    userId: Int64;
  };

  type deleteOrgParams = {
    id: Int64;
  };

  type getChildrenByParentIdParams = {
    parentId: Int64;
  };

  type moveOrgParams = {
    id: Int64;
    newParentId: Int64;
  };

  type getDeptByIdParams = {
    id: Int64;
  };

  type deleteDeptParams = {
    id: Int64;
  };

  type getChildDeptIdsParams = {
    deptId: Int64;
  };

  type getPostByIdParams = {
    id: Int64;
  };

  type deletePostParams = {
    id: Int64;
  };

  type getPostPageParams = {
    current?: number;
    size?: number;
    name?: string;
    code?: string;
    status?: number;
  };

  type changePostStatusParams = {
    id: Int64;
    status: number;
  };

  type getOnlineUsersParams = {
    current?: number;
    pageSize?: number;
    userName?: string;
    loginIp?: string;
    browser?: string;
    os?: string;
  };

  type getOperationLogByIdParams = {
    id: Int64;
  };

  type cleanExpiredLogsParams = {
    days?: number;
  };

  type deleteLogsByTimeRangeParams = {
    startTime: string;
    endTime: string;
  };

  type exportOperationLogsParams = {
    operationType?: string;
    startTime?: string;
    endTime?: string;
  };

  type getMyOperationLogPageParams = {
    current?: number;
    size?: number;
    operationType?: string;
    requestMethod?: string;
    requestUrl?: string;
    executionTimeMin?: number;
    executionTimeMax?: number;
    startTime?: string;
    endTime?: string;
  };

  type checkUserOnlineParams = {
    userId: Int64;
  };

  type kickUserParams = {
    userId: Int64;
  };

  type getUserSessionParams = {
    userId: Int64;
  };

  type revokeCurrentUserSessionParams = {
    sessionId: string;
  };

  type revokeCurrentUserDeviceParams = {
    deviceId: string;
  };

  type listAdminUserSessionsParams = {
    userId: Int64;
  };

  type kickAdminUserSessionParams = {
    userId: Int64;
    sessionId: string;
  };

  type kickAdminUserAllSessionsParams = {
    userId: Int64;
  };

  type listAdminUserDevicesParams = {
    userId: Int64;
  };

  type kickAdminUserDeviceParams = {
    userId: Int64;
    deviceId: string;
  };

  type listAdminUserFrozenDevicesParams = {
    userId: Int64;
  };

  type freezeAdminUserDeviceParams = {
    userId: Int64;
    deviceId: string;
    freezeSeconds?: number;
    reason?: string;
  };

  type unfreezeAdminUserDeviceParams = {
    userId: Int64;
    deviceId: string;
  };

  type setAdminUserDeviceLimitParams = {
    userId: Int64;
    maxDevices: number;
  };

  type clearAdminUserDeviceLimitParams = {
    userId: Int64;
  };
}
