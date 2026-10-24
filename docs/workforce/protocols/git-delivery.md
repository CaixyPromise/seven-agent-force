# Git 与交付集成协议

Git不是Workforce自定义RPC：Project源码与结果使用Git clone/fetch/push协议，平台DeliveryRequest／DeliveryAttempt将冻结结果、授权与远端操作串起来。协议运输支持HTTP、HTTPS、SSH，以及仅本地测试的LOCAL_BARE。远端Git SHA是string，不是数据库ID。

普通外部调用者只选择deliveryPreference，不能在Open请求中注入带凭据cloneURL。用户在正常Project表单保存凭据后产生新的ProjectVersion／凭据绑定；Git交付读取对应不可变版本。凭据不从Agent工具参数取出或暴露。

## 平台方法与参数

完整URL、DTO与字段见 [operator HTTP](http-operator.md) 中 projects、delivery-targets、delivery-requests，以及 [Worker HTTP](http-worker.md) 的delivery-attempts。当前状态/version参数以具体DTO字段为准；恢复先读取request历史再走受版本约束的retry/reconcile，不手写数据库状态。

Agent平台工具 `request_git_delivery(arguments)`：arguments为闭合object，summary:string必填，1–2048 bytes。此方法提出交付请求，平台冻结结果／完成Review／使用既有授权或等待人工决定；不会给Agent返回Git密码，不在Agent内直接push。

结果冻结对象、Git目标与交付计划字段见类型索引中的 delivery/domain.Target、Request、Attempt 和 workspaceinput域DTO。后处理重试不重跑原Agent；未知push/MR结果优先只读回查远端branch SHA与已有MR，防止重复。目标branch、force、MR/Draft选择由实际授权与计划决定，不给客户端隐式merge权限。

## 远端API边界

GitLab、GitHub与Gitea使用现有provider adapter创建／查询MR或PR；实际远端HTTP参数、鉴权和Draft命名映射见下面实现。标准API对象并非Workforce的另一套公开身份。对接新Git平台需实现已有Delivery端口，并覆盖远端读回查、branch结果一致性、未知结果与幂等；不要将Provider原始错误body直接返回用户。

源码：[Delivery域](../../../seven-agent-force-server/internal/app/workforce/delivery/domain/model.go)、[Delivery基础设施](../../../seven-agent-force-server/internal/app/workforce/delivery/infrastructure)、[平台Git工具](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go)。

## 已实现的远端Draft API参数

base：GitLab `/api/v4/projects/:urlEncodedRepositoryPath`，GitHub `/repos/:owner/:repo`（Enterprise为/api/v3/repos），Gitea `/api/v1/repos/:owner/:repo`。所有path参数为string。平台Provider token由受保护凭据绑定提供，不从外部Task body接收。

| 平台／method | path | 参数与类型 | 鉴权／结果 |
| --- | --- | --- | --- |
| GitLab POST | base/merge_requests | source_branch/target_branch/title/description:string；remove_source_branch:boolean=false | PRIVATE-TOKEN；title使用Draft:；核验id/iid/web_url |
| GitLab GET | base/merge_requests | query scope:string=all、source_branch/target_branch:string | 查marker与分支身份；核验Draft |
| GitHub POST | base/pulls | head/base/title/body:string；draft:boolean=true | Bearer；X-GitHub-Api-Version:2026-03-10；核验id/number/html_url |
| GitHub GET | base/pulls | query state:string=all、head:string(owner:branch)、base:string | 回查已有Draft PR |
| Gitea POST | base/pulls | head/base/title/body:string | Authorization:token；title WIP:标记，响应仍验证Draft |
| Gitea GET | base/pulls | query state:string=all、base_branch:string | 逐项核验分支与marker |
| GitLab GET | base/repository/branches/:branch | branch:string path | 读取base commit SHA |
| GitHub/Gitea GET | base/branches/:branch | branch:string path | 读取base commit SHA |

ReviewPublication内部投影：providerReviewId/providerNumber/webURL/observedBase:string（真实Go字段和JSON公开投影见类型索引）。远端返回的id/number可以是number或string，适配器转换为opaque string；不可和Workforce实体ID混用。

对应实现：[review_provider.go](../../../seven-agent-force-server/internal/app/workforce/delivery/infrastructure/review_provider.go)。
