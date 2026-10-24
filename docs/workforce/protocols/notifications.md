# 任务通知与Framework通道协议

Task通知复用active server的Framework notification模块，包含站内、SMTP、飞书／企业微信应用、固定群Webhook和受约束HTTP Connector。它与 [Open Application公共Webhook](webhook.md) 是不同契约。下面记录Workforce生产端与驱动接入端，不重复Framework通用管理CRUD。

## Workforce方法与参数

HTTP：GET `/api/workforce/task-notifications/readiness`；GET/PUT `/api/workforce/tasks/:taskId/notification-subscription`；GET `/api/workforce/tasks/:taskId/notification-records`。taskId:string(ID)。PUT body：enabled:boolean、events:FactType[]、channels:ChannelSelection[]、expectedVersion:string(int64)，严格JSON，≤64 KiB。完整嵌套字段、返回DTO、权限与冲突错误见 [operator方法参考](http-operator.md)。

Go开发端口：

| 方法 | 参数类型 | 返回类型 |
| --- | --- | --- |
| RecordTransition | context.Context、pgx.Tx、TaskNotificationTransition | TransitionDisposition、error |
| CreateSubscription | context.Context、pgx.Tx、tenantID/taskID/creatorID:int64、SubscriptionCommand | Subscription、bool（创建结果）、error |
| ResolveInternalLink | deepLink:string | absoluteURL:string、available:bool、reasonCode:string |

Transition、ChannelSelection、Subscription等完整字段见 [类型索引](wire-types.md) 的tasknotification/domain章节。FactType为TASK_ACTION_REQUIRED / TASK_INTERRUPTED / TASK_COMPLETED / DELIVERY_COMPLETED；通道选择IN_APP / EMAIL_USER / FEISHU_DIRECT / WECOM_DIRECT / GROUP_ROUTE由平台解析身份与route，不让调用者提交任意Provider body。事实与outbox在当前业务事务中持久化；事实去重与发送结果分别记录。

## Notification Driver扩展端口

| 方法 | 参数类型 | 返回 |
| --- | --- | --- |
| Send | ctx:context.Context、message:DriverMessage | error |
| SendResult | ctx:context.Context、message:DriverMessage | DriverResult、error |
| Driver | channelType:string | ChannelDriver |
| Capabilities | channelType:string | DriverCapabilities |

DriverMessage字段级类型：

| 字段 | Go类型 |
| --- | --- |
| Channel | notification/domain.Channel（通道配置对象） |
| SecretPlain | string，只在驱动边界提供，不记录／返回 |
| IdentityKind / Target / Subject / Text / HTML / Markdown | string |
| Variables / ProviderParams | map[string]any |
| DeliveryID / EventKey / Category / Priority / TraceID / DeepLink | string |
| PrimaryAction | *NotificationAction，平台冻结动作 |
| ExternalActionURL / Presentation | string |
| Mention | *DriverMention{Provider:string,Subject:string,Label:string}，平台解析 |
| Probe | bool，受权限控制的非持久连接探测 |

DriverResult：Status/FailureClass/ProviderReference/Diagnostic:string；Retryable:bool；ProviderError:*ProviderError{Provider:string,HTTPStatus:int,Code:string,Message:string,LogID:string}。ProviderError只允许经过sanitize的探测诊断，不持久化raw body。ProviderAccepted是上游接收，不等于用户已阅读；发送响应未知须保留UNKNOWN，不能盲目换deliveryId重发。

## SMTP与企业应用方法

SMTP配置：host/username/from/timeout/tlsServerName:string，port:int，useTls/startTls/skipVerify:bool；SecretPlain为通道密码。发送目标message.Target:string，Subject/Text/HTML:string。驱动执行连接、TLS、认证、MAIL FROM、RCPT TO、DATA；DATA之后响应丢失返回UNKNOWN，不能当成“未发送”。

| Provider HTTP方法 | 参数与类型 | 返回消费字段 |
| --- | --- | --- |
| 飞书 POST `/open-apis/auth/v3/tenant_access_token/internal` | app_id/app_secret:string | code:integer、tenant_access_token:string、expire:integer |
| 飞书 POST `/open-apis/im/v1/messages` | query receive_id_type:string=open_id/chat_id；Bearer；body receive_id/msg_type/content/uuid:string | code:integer、data.message_id:string |
| 企业微信 GET `/cgi-bin/gettoken` | query corpid/corpsecret:string | errcode:integer、access_token:string、expires_in:integer |
| 企业微信 POST `/cgi-bin/message/send` | query access_token:string；body touser/msgtype:string、agentid:integer、text:{content:string}或template_card:object | errcode:integer、msgid/invaliduser/unlicenseduser:string |

飞书content是序列化JSON **string**，不是body中的直接object；text或interactive由冻结Presentation决定；uuid来自同一DeliveryID。企业微信template_card含card_type:string、source:{desc:string}、main_title:{title,desc:string}，可包含card_action:{type:integer,url:string}；mention字段由平台解析。token失效按驱动受控刷新，禁止将token打印或持久化进接收记录。

HTTPConnectorConfig：endpointUrl/egressPolicyRef/method/idempotencyHeader:string；authentication:{mode:string,secretRef?:string}；fieldMappings:array{source:string,target:string}；headerAllowlist:string[]；timeoutMilliseconds:integer；successStatusCodes:integer[]。固定群Webhook非秘密配置仅timeoutMilliseconds:integer、successStatusCodes:integer[]；完整URL和签名材料存受保护凭据。业务调用者不能改目标URL、SecretRef、raw body或平台冻结PrimaryAction。

源码：[Task facade](../../../seven-agent-force-server/internal/app/workforce/tasknotification/facade/contracts.go)、[Driver端口](../../../seven-agent-force-server/internal/app/notification/domain/driver.go)、[SMTP](../../../seven-agent-force-server/internal/app/notification/infrastructure/drivers.go)、[企业应用](../../../seven-agent-force-server/internal/app/notification/infrastructure/enterprise_app.go)、[HTTPConnector](../../../seven-agent-force-server/internal/app/notification/domain/http_connector.go)、[固定Webhook](../../../seven-agent-force-server/internal/app/notification/domain/webhook_profile.go)。
