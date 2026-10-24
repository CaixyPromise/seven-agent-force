# 协议文档生成与校验

工具只读取已跟踪源码与Go类型信息，不启动API/Worker、读取运行配置／凭据或执行Python Adapter。需要本机Go与Python3。当前指南随源码快照一起维护；源码升级时先检查真实挂载和消费者，更新快照与语义指南。

## 生成

在仓库根目录创建被忽略的工作文件目录，再进入active server生成Go export index（编译类型元数据，不运行业务测试）。

```sh
mkdir -p .local/protocol-docs-20261006
cd seven-agent-force-server
go list -deps -export -json ./internal/app/workforce/... ./internal/worker/... ./internal/mcpserver ./internal/common/response ./cmd/seven-worker-cli ./cmd/seven-workerctl ./cmd/seven-provider-proxy > ../.local/protocol-docs-20261006/go-packages.json
go run ../docs/workforce/protocols/tools/extract.go ../.local/protocol-docs-20261006/go-packages.json ../.local/protocol-docs-20261006/contracts.json
cd ..
python3 docs/workforce/protocols/tools/render.py .local/protocol-docs-20261006/contracts.json
python3 docs/workforce/protocols/tools/check.py
git diff --check
```

生成页：http-open/http-operator/http-worker/http-provider、wire-types、python-sdk、cli；机器清单contracts.json。人工维护页：README、common及各协议指南。更新数量时同步check.py的快照预期与README。

## 提取方法与边界

extract.go用AST和go/types解析controller注册与同包辅助调用，记录字面URL、权限、path/query/header/form参数、decode的请求DTO、success的响应类型与直接map字段；将命名类型、匿名对象与嵌套类型递归展开。按当前module挂载，排除未挂载的executionaccess旧machine controller。

render.py补充已核实的动态query helper参数（日期范围、optionalQueryID）；用实际JSON tag区分native JSON与HTTP int64归一化。Python SDK仅AST提取签名和dataclass，不import。CLI按实际flag解析函数列出定义，不读取环境值。

这不是OpenAPI规范文件，也不执行域校验推导：omitempty不等于业务可选；入口错误列表不等于全域枚举；RawMessage/map[string]any/Provider透传不强行闭合；any postprocess列表和custom MarshalJSON需要人工指南补足。新增特殊wrapper、动态路由、运行时序列化规则时必须更新提取器或显式说明，不能只信生成计数。

check.py校验231路由快照与分组、重复方法、类型引用闭环、25个Adapter方法，以及所有本地文件／显式anchor／源码行号。只验证文档结构，不替代接口集成或真实验收。
