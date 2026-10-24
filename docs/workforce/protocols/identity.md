# 继承的OAuth／OIDC与企业身份开发端口

Workforce沿用Framework密码登录、用户会话、OAuth／OIDC、组织身份与授权。Application API Key不是OAuth access token，Worker token也不是用户session。这些能力在active server的login/external_login模块；本页记录对接Provider的开发方法和参数，231个Workforce路由清单不包含Framework全量管理API。

## OAuth／OIDC Driver方法

| 方法 | 参数（Go类型） | 返回 |
| --- | --- | --- |
| Code | 无 | string |
| Capabilities | 无 | domain.ProviderCapability |
| BuildAuthorizationURL | ctx:context.Context、provider:domain.Provider、request:AuthorizationRequest | string URL、error |
| ExchangeCode | ctx、provider、request:TokenExchangeRequest | *TokenExchangeResult、error |
| ResolveProfile | ctx、provider、tokens:TokenExchangeResult | *domain.ExternalProfile、error |
| RefreshToken | ctx、provider、tokenSet:domain.TokenSet | *domain.TokenSet、error，按Provider能力 |
| RevokeToken | ctx、provider、tokenSet:domain.TokenSet | error，按Provider能力 |
| Discover（OIDCDriver） | ctx、expectedIssuer:string | DiscoveryDocument、error |
| ValidateIDToken | ctx、raw:string、options:OIDCValidationOptions | OIDCClaims、error |

AuthorizationRequest字段：State/Nonce/CodeChallenge/CodeChallengeMethod/RedirectURI/Issuer:string，Scopes:[]string。

TokenExchangeRequest：Code/State/CodeVerifier/RedirectURI/ClientSecret/Nonce/ExpectedIssuer/CallbackIssuer:string，Scopes:[]string。TokenExchangeResult：TokenSet:domain.TokenSet，RawTokenResponse/ExpectedIssuer/CallbackIssuer/ExpectedNonce:string。RawTokenResponse仅内部处理，不记录／暴露。

TokenSet：accessToken/refreshToken/idToken/tokenType/rawProfile:string（可省略）、scopes:string[]、expiresAt:*time.Time。DiscoveryDocument：issuer/authorization_endpoint/token_endpoint/userinfo_endpoint/jwks_uri:string。OIDCValidationOptions：Issuer/Audience/Nonce/JWKSURI:string。OIDCClaims：Issuer/Subject/Nonce/Email/Name/Picture/Raw:string，Audience:[]string，ExpiresAt/IssuedAt:time.Time，EmailVerified:bool。

Provider配置对象的端点、ClientID、Scopes、能力和受保护凭据来自管理配置，实际字段见下方domain/model.go；不能让登录callback自由替换Issuer/JWKS/token endpoint。OIDC discovery上限64 KiB，验证Issuer与endpoint origin。OAuth authorization_code、PKCE、state、nonce和ID token验证分别执行，不能只拿code换到token就建立登录态。具体Provider能否refresh/revoke以Capabilities与实现为准。

## 企业身份Adapter方法

| 方法 | 参数类型 | 返回 |
| --- | --- | --- |
| Provider | 无 | domain.EnterpriseProvider |
| Capabilities | 无 | EnterpriseProtocolCapabilities |
| BuildAuthorizationURL | connection:EnterpriseConnection、state/nonce/codeChallenge/redirectURI:string | string URL、error |
| ExchangeAndResolve | ctx:context.Context、connection、clientSecret/code/codeVerifier/redirectURI:string | EnterpriseResolvedSubject、error |
| Probe | ctx、connection、clientSecret:string | error |
| SendPersonalTest | ctx、connection、clientSecret/subject:string | error |
| DiscoverGroups（支持的扩展） | ctx、connection、secret:string | []EnterpriseDiscoveredGroup、error |

EnterpriseResolvedSubject：Subject/SubjectKind/Nonce:string；EnterpriseDiscoveredGroup：DisplayName/Description/Target:string。EnterpriseProtocolCapabilities：supportsAuthorizationCode/supportsPkce/supportsOidcNonce/supportsUserInfo:boolean。EnterpriseConnection为组织下配置与version对象，包含ID/OrganizationID:int64、Provider enum、AppID/CorpID/AgentID:string、secret引用／密文和状态，不由用户callback直接提供。

| 已实现上游操作 | 参数类型 |
| --- | --- |
| 飞书 POST `/open-apis/authen/v2/oauth/token` | grant_type/client_id/client_secret/code/redirect_uri:string |
| 飞书 GET `/open-apis/authen/v1/user_info` | Authorization:string(Bearer用户token)；消费data.open_id:string |
| 飞书 GET `/open-apis/im/v1/chats?page_size=100` | Bearer tenant token；消费data.items的chat_id/name/description:string |
| 企业微信浏览器GET `/wwopen/sso/qrConnect` | appid/agentid/redirect_uri/state:string |
| 企业微信 GET `/cgi-bin/auth/getuserinfo` | access_token/code:string；消费UserId:string |

企业微信Adapter未声明PKCE/OIDC nonce支持，不能因为内部端口接受相关参数就认为全部Provider已实现。通知身份绑定与登录授权独立；解绑／撤销需要正常业务流程，不能伪造用户或临时改数据库。

实现入口：[OAuth Driver端口及类型](../../../seven-agent-force-server/internal/app/external_login/infrastructure/drivers/registry.go)、[OAuth域对象](../../../seven-agent-force-server/internal/app/external_login/domain/model.go)、[OIDC实现](../../../seven-agent-force-server/internal/app/external_login/infrastructure/drivers/oidc_generic.go)、[企业端口](../../../seven-agent-force-server/internal/app/external_login/application/enterprise_service.go)、[企业Adapter](../../../seven-agent-force-server/internal/app/external_login/enterprise_provider.go)、[企业对象](../../../seven-agent-force-server/internal/app/external_login/domain/enterprise_identity.go)、[现有HTTP注册](../../../seven-agent-force-server/internal/app/external_login/module.go)、[HTTP facade参数](../../../seven-agent-force-server/internal/app/external_login/facade/types.go)。
