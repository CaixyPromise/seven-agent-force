-- +goose Up
CREATE TABLE IF NOT EXISTS sys_user (
  id BIGINT NOT NULL AUTO_INCREMENT COMMENT '用户ID',
  userAccount VARCHAR(30) NOT NULL COMMENT '账号',
  nickName VARCHAR(30) NOT NULL COMMENT '昵称',
  password VARCHAR(100) DEFAULT '' COMMENT '兼容 Java 用户表密码字段，Go 密码凭证写 credential 模块',
  status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0正常 1停用/锁定',
  creatorId BIGINT DEFAULT NULL,
  updaterId BIGINT DEFAULT NULL,
  userPhone VARCHAR(11) DEFAULT NULL,
  userEmail VARCHAR(254) NOT NULL DEFAULT '',
  email_verified_at DATETIME(3) NULL,
  email_version BIGINT NOT NULL DEFAULT 1 CHECK (email_version > 0),
  email_verification_source VARCHAR(64) NOT NULL DEFAULT '',
  userGender TINYINT(1) NOT NULL DEFAULT 0,
  userAvatar VARCHAR(1024) DEFAULT NULL,
  userProfile VARCHAR(1024) DEFAULT NULL,
  unsealTime DATETIME DEFAULT NULL,
  deletionTime DATETIME DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_account_deleted (userAccount, isDeleted),
  KEY idx_user_phone (userPhone),
  KEY idx_user_email (userEmail),
  KEY idx_user_verified_email (userEmail, email_verified_at),
  KEY idx_user_status (status, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统用户表';

CREATE TABLE IF NOT EXISTS sys_org (
  id BIGINT NOT NULL COMMENT '组织ID',
  code VARCHAR(64) NOT NULL COMMENT '组织编码',
  name VARCHAR(128) NOT NULL COMMENT '组织名称',
  parentId BIGINT NOT NULL DEFAULT 0 COMMENT '父组织ID',
  hierarchy VARCHAR(500) DEFAULT NULL COMMENT '组织层级路径',
  level INT DEFAULT 0 COMMENT '组织层级',
  status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0正常 1停用',
  sortOrder INT NOT NULL DEFAULT 0 COMMENT '排序',
  leaderUserId BIGINT DEFAULT NULL COMMENT '负责人',
  creatorId BIGINT DEFAULT NULL,
  updaterId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_org_code (code),
  KEY idx_org_parent (parentId, isDeleted),
  KEY idx_org_status (status, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统组织表';

CREATE TABLE IF NOT EXISTS sys_dept (
  id BIGINT NOT NULL COMMENT '部门ID',
  name VARCHAR(128) NOT NULL COMMENT '部门名称',
  code VARCHAR(64) NOT NULL COMMENT '部门编码',
  orgId BIGINT NOT NULL COMMENT '组织ID',
  parentId BIGINT NOT NULL DEFAULT 0 COMMENT '父部门ID',
  leaderUserId BIGINT DEFAULT NULL COMMENT '负责人',
  status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0正常 1停用',
  sortOrder INT NOT NULL DEFAULT 0 COMMENT '排序',
  hierarchy VARCHAR(512) DEFAULT NULL COMMENT '层级路径',
  level INT NOT NULL DEFAULT 1 COMMENT '层级',
  creatorId BIGINT DEFAULT NULL,
  updaterId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_dept_code_deleted (code, isDeleted),
  KEY idx_dept_parent (parentId, isDeleted),
  KEY idx_dept_org (orgId, isDeleted),
  KEY idx_dept_status (status, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统部门表';

CREATE TABLE IF NOT EXISTS sys_post (
  id BIGINT NOT NULL COMMENT '岗位ID',
  code VARCHAR(64) NOT NULL COMMENT '岗位编码',
  name VARCHAR(128) NOT NULL COMMENT '岗位名称',
  deptId BIGINT NOT NULL DEFAULT 0 COMMENT '部门ID',
  orgId BIGINT NOT NULL DEFAULT 0 COMMENT '组织ID',
  hierarchy VARCHAR(500) DEFAULT NULL COMMENT '岗位层级路径',
  level INT DEFAULT 0 COMMENT '岗位层级',
  sortOrder INT NOT NULL DEFAULT 0 COMMENT '排序',
  status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0正常 1停用',
  remark VARCHAR(512) DEFAULT NULL COMMENT '备注',
  creatorId BIGINT DEFAULT NULL,
  updaterId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY idx_post_dept (deptId, isDeleted),
  KEY idx_post_org (orgId, isDeleted),
  KEY idx_post_status (status, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统岗位表';

CREATE TABLE IF NOT EXISTS sys_user_role (
  id BIGINT NOT NULL AUTO_INCREMENT,
  userId BIGINT NOT NULL,
  roleId BIGINT NOT NULL,
  creatorId BIGINT DEFAULT NULL,
  updaterId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_role (userId, roleId),
  KEY idx_user_role_user (userId, isDeleted),
  KEY idx_user_role_role (roleId, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户角色关系表';

CREATE TABLE IF NOT EXISTS sys_user_org (
  id BIGINT NOT NULL AUTO_INCREMENT,
  userId BIGINT NOT NULL,
  orgId BIGINT NOT NULL,
  isPrimary TINYINT(1) NOT NULL DEFAULT 0,
  creatorId BIGINT DEFAULT NULL,
  updaterId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_org (userId, orgId),
  KEY idx_user_org_user (userId, isDeleted),
  KEY idx_user_org_org (orgId, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户组织关系表';

CREATE TABLE IF NOT EXISTS sys_user_dept (
  id BIGINT NOT NULL AUTO_INCREMENT,
  userId BIGINT NOT NULL,
  deptId BIGINT NOT NULL,
  isPrimary TINYINT(1) NOT NULL DEFAULT 0,
  creatorId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_dept (userId, deptId),
  KEY idx_user_dept_user (userId),
  KEY idx_user_dept_dept (deptId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户部门关系表';

CREATE TABLE IF NOT EXISTS sys_user_position (
  id BIGINT NOT NULL AUTO_INCREMENT,
  userId BIGINT NOT NULL,
  postId BIGINT NOT NULL,
  isPrimary TINYINT(1) NOT NULL DEFAULT 0,
  creatorId BIGINT DEFAULT NULL,
  updaterId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_post (userId, postId),
  KEY idx_user_position_user (userId, isDeleted),
  KEY idx_user_position_post (postId, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户岗位关系表';

CREATE TABLE IF NOT EXISTS sys_post_role (
  id BIGINT NOT NULL AUTO_INCREMENT,
  postId BIGINT NOT NULL,
  roleId BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_post_role (postId, roleId),
  KEY idx_post_role_post (postId),
  KEY idx_post_role_role (roleId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='岗位角色关系表';

CREATE TABLE IF NOT EXISTS sys_role (
  id BIGINT NOT NULL,
  name VARCHAR(64) NOT NULL COMMENT '角色名称',
  code VARCHAR(100) NOT NULL COMMENT '角色编码',
  dataScope TINYINT NOT NULL DEFAULT 1 COMMENT '数据范围',
  status TINYINT NOT NULL DEFAULT 0 COMMENT '状态',
  type TINYINT NOT NULL DEFAULT 0 COMMENT '角色类型',
  hierarchy VARCHAR(500) DEFAULT NULL COMMENT '角色层级路径',
  level INT DEFAULT 0 COMMENT '角色层级',
  sortOrder INT DEFAULT 0 COMMENT '排序',
  remark VARCHAR(500) DEFAULT NULL COMMENT '备注',
  creatorId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updaterId BIGINT DEFAULT NULL,
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY idx_role_code (code),
  KEY idx_role_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色信息表';

CREATE TABLE IF NOT EXISTS sys_permission (
  id BIGINT NOT NULL,
  code VARCHAR(100) NOT NULL COMMENT '权限编码',
  name VARCHAR(100) NOT NULL COMMENT '权限名称',
  resourceType VARCHAR(30) NOT NULL DEFAULT 'API' COMMENT '资源类型',
  method VARCHAR(10) DEFAULT NULL,
  path VARCHAR(255) DEFAULT NULL,
  status TINYINT NOT NULL DEFAULT 0,
  description VARCHAR(255) DEFAULT NULL,
  creatorId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updaterId BIGINT DEFAULT NULL,
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_permission_code (code),
  KEY idx_permission_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='权限资源表';

CREATE TABLE IF NOT EXISTS sys_menu (
  id BIGINT NOT NULL,
  name VARCHAR(64) NOT NULL COMMENT '菜单名称',
  parentId BIGINT NOT NULL DEFAULT 0,
  sortOrder INT DEFAULT 0,
  path VARCHAR(200) DEFAULT '',
  component VARCHAR(255) DEFAULT NULL,
  icon VARCHAR(100) DEFAULT NULL,
  type CHAR(1) NOT NULL,
  permission VARCHAR(100) DEFAULT NULL,
  isFrame TINYINT(1) DEFAULT 0,
  isCache TINYINT(1) DEFAULT 0,
  visible TINYINT(1) DEFAULT 1,
  hierarchy VARCHAR(500) DEFAULT NULL,
  level INT DEFAULT 0,
  status TINYINT NOT NULL DEFAULT 0,
  remark VARCHAR(255) DEFAULT NULL,
  creatorId BIGINT DEFAULT NULL,
  createTime DATETIME DEFAULT CURRENT_TIMESTAMP,
  updaterId BIGINT DEFAULT NULL,
  updateTime DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY idx_menu_parent (parentId),
  KEY idx_menu_status (status),
  KEY idx_menu_type (type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='菜单权限表';

CREATE TABLE IF NOT EXISTS sys_menu_permission (
  id BIGINT NOT NULL,
  menuId BIGINT NOT NULL,
  permissionId BIGINT NOT NULL,
  creatorId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_menu_permission (menuId, permissionId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='菜单权限映射表';

CREATE TABLE IF NOT EXISTS sys_role_permission (
  id BIGINT NOT NULL,
  roleId BIGINT NOT NULL,
  permissionId BIGINT NOT NULL,
  creatorId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_role_permission (roleId, permissionId),
  KEY idx_role_permission_role (roleId),
  KEY idx_role_permission_permission (permissionId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色权限关系表';

CREATE TABLE IF NOT EXISTS sys_role_menu (
  id BIGINT NOT NULL,
  roleId BIGINT NOT NULL,
  menuId BIGINT NOT NULL,
  updaterId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_role_menu (roleId, menuId),
  KEY idx_role_menu_role (roleId),
  KEY idx_role_menu_menu (menuId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色菜单关系表';

CREATE TABLE IF NOT EXISTS sys_role_dept (
  id BIGINT NOT NULL,
  roleId BIGINT NOT NULL,
  deptId BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_role_dept (roleId, deptId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色部门数据范围关系表';

CREATE TABLE IF NOT EXISTS sys_user_permission (
  id BIGINT NOT NULL AUTO_INCREMENT,
  userId BIGINT NOT NULL,
  permissionId BIGINT NOT NULL,
  type TINYINT(1) NOT NULL DEFAULT 1,
  expireTime DATETIME DEFAULT NULL,
  source VARCHAR(100) DEFAULT NULL,
  grantedBy BIGINT DEFAULT NULL,
  creatorId BIGINT DEFAULT NULL,
  updaterId BIGINT DEFAULT NULL,
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_permission (userId, permissionId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户临时权限表';

-- +goose Down
DROP TABLE IF EXISTS sys_user_permission;
DROP TABLE IF EXISTS sys_role_dept;
DROP TABLE IF EXISTS sys_role_menu;
DROP TABLE IF EXISTS sys_role_permission;
DROP TABLE IF EXISTS sys_menu_permission;
DROP TABLE IF EXISTS sys_menu;
DROP TABLE IF EXISTS sys_permission;
DROP TABLE IF EXISTS sys_role;
DROP TABLE IF EXISTS sys_post_role;
DROP TABLE IF EXISTS sys_user_position;
DROP TABLE IF EXISTS sys_user_dept;
DROP TABLE IF EXISTS sys_user_org;
DROP TABLE IF EXISTS sys_user_role;
DROP TABLE IF EXISTS sys_post;
DROP TABLE IF EXISTS sys_dept;
DROP TABLE IF EXISTS sys_org;
DROP TABLE IF EXISTS sys_user;
