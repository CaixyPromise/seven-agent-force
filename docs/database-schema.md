# PostgreSQL 结构导出与初始化

Workforce 使用 PostgreSQL。空库安装、纯结构恢复和既有库升级是不同流程。本仓库提供初始化迁移和无业务数据的规范结构参考，不分发运行数据库、用户账号、任务、模型凭据或实际迁移记录。

## 1. 全新安装：基线，再升级链

先创建专用空数据库，配置 `datasource.driver=postgres` 及对应连接。使用环境变量或私有配置提供连接信息，不把密码写入命令历史或仓库。基线只能应用于没有业务表的新库。

从源码构建的命令为 `go run ./cmd/seven-agent-force-server migrate`（工作目录为 Server）；发布包使用 `bin/seven-agent-force-server migrate`。命令的参数必须放在 up/status/version 等子操作之前。

假设发布包在 `/opt/seven-agent-force/current`，配置在 `/etc/seven-agent-force/api`：

```sh
/opt/seven-agent-force/current/bin/seven-agent-force-server migrate   --home /opt/seven-agent-force/current   --config-dir /etc/seven-agent-force/api   --dir /opt/seven-agent-force/current/migrations/postgres-baseline up

/opt/seven-agent-force/current/bin/seven-agent-force-server migrate   --home /opt/seven-agent-force/current   --config-dir /etc/seven-agent-force/api up

/opt/seven-agent-force/current/bin/seven-agent-force-server migrate   --home /opt/seven-agent-force/current   --config-dir /etc/seven-agent-force/api status
```

首次操作运行专用基线，第二次切回常规 PostgreSQL 升级链。基线含公开定义的系统权限/菜单/模板种子，并登记其覆盖的迁移版本；不包含运行实例的业务记录。不要手写 Goose 版本号来跳过失败迁移。

当前 PostgreSQL 的 `inspect/bootstrap` 自动编排未实现；这两个子操作属于 MySQL bootstrap 服务，不应用于 Workforce 安装。PostgreSQL 使用这里已验证的显式 baseline → up → status 路径。

## 2. 纯结构参考：schema/postgres.sql

规范 DDL `seven-agent-force-server/schema/postgres.sql`（发布包内为 `schema/postgres.sql`） 从干净迁移初始化的 PostgreSQL 生成。它只含 public schema 的表、列、约束、索引、函数、序列定义和触发器，不含业务行、序列当前值、所有者、ACL、运行配置或 Goose 版本记录。PG16/18 支持范围应以对应版本的验证报告为准。

用 libpq 环境或服务文件指向一个明确的新空库后，可以执行：

```sh
psql -X -v ON_ERROR_STOP=1 -f seven-agent-force-server/schema/postgres.sql
```

该文件用于结构审阅、对比和结构恢复，**不是完整应用安装包**：它不包含系统种子，也没有迁移账本。恢复结构后不能直接运行整条历史迁移。全新可用实例请按上一节安装，再走正常初始化与登录流程。

## 3. 只读导出自己的实例

需要 pg_dump 客户端版本不低于目标数据库主版本，以及 Python 3。通过现有 libpq 配置选择目标，不传带密码的连接 URL 到命令参数：

```sh
python3 scripts/database/export-schema.py --output /path/to/new-schema.sql
```

导出器强制源会话只读，使用 schema-only、no-owner、no-privileges、no-comments、no-security-labels、no-tablespaces、no-publications、no-subscriptions，并排除迁移账本。输出权限0600，拒绝覆盖既有文件。它去除 pg_dump 版本/客户端标记和不兼容 PG16 的会话参数；保留 SQL 对象语义。

这不是完整备份工具。只导出 public schema，不导出角色、数据库级区域设置、其他schema、扩展安装、外部存储、加密密钥或恢复策略。外部依赖和默认collation需在目标环境独立配置。PG18实例可以具有PG16不支持的对象语法；不要把任意PG18现场导出当作PG16便携安装文件。

字段类型、只读预检、失败处理及统一迁移策略见 [存储契约与升级](database-storage-contract.md)。

## 4. 对比与升级

结构对比应逐项检查表/字段、类型、默认值、可空性、约束、索引、函数、序列和触发器。不要只比较表数量或Goose最大版本，也不要把系统生成的NOT NULL约束名、隐式类型转换和索引名字变化全部当作缺表。

既有运行库可能保留历史类型宽度、旧索引名或旧状态约束；当前代码迁移是新安装的规范，现场结构导出是观测事实。差异应分类评估，不能为了文本一致弱化新库校验，也不能未验证就收窄已有数据的类型。升级先备份并审阅实际连接目标，只应用经批准的前向迁移。纯结构导出不授权修改源库。

## 5. 维护本文件

更新迁移后在独立空库执行完整链，再重新生成规范DDL。分别测试纯DDL恢复与完整迁移安装；前者验证零业务行，后者还验证必要静态种子、迁移账本和重复up。导出器单元测试：

```sh
python3 -m unittest discover -s scripts/database -p 'test_*.py'
```
