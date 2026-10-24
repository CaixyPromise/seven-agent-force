-- +goose Up
-- Goal-6.3 mirrors MySQL: normal delivery lists remain content-free, while
-- strictly scoped diagnostic evidence and short-lived secret envelopes are
-- retained separately.

ALTER TABLE "public"."sysNotificationDelivery"
    ADD COLUMN IF NOT EXISTS "contentTier" character varying(32) NOT NULL DEFAULT 'SENSITIVE';

CREATE TABLE IF NOT EXISTS "public"."sysNotificationDeliveryEphemeralContent" (
    "id" BIGINT PRIMARY KEY,
    "deliveryId" character varying(128) NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    ciphertext text NOT NULL,
    edek text NOT NULL,
    "wrapKeyRef" character varying(128) NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updateTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "ukNotificationDeliveryEphemeralContentDelivery" UNIQUE ("deliveryId")
);
CREATE INDEX IF NOT EXISTS "idxNotificationDeliveryEphemeralContentScopeExpiry"
    ON "public"."sysNotificationDeliveryEphemeralContent" ("scopeId", "expiresAt");

CREATE TABLE IF NOT EXISTS "public"."sysNotificationDeliveryDiagnosticAudit" (
    "id" BIGINT PRIMARY KEY,
    "scopeId" character varying(128) NOT NULL,
    "deliveryId" character varying(128) NOT NULL,
    "actorId" BIGINT NOT NULL,
    "contentTier" character varying(32) NOT NULL,
    "reasonCode" character varying(32) NOT NULL,
    "ticketReference" character varying(128),
    "resultCode" character varying(48) NOT NULL,
    "traceId" character varying(128),
    "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS "idxNotificationDeliveryDiagnosticAuditScopeTime"
    ON "public"."sysNotificationDeliveryDiagnosticAudit" ("scopeId", "createTime");
CREATE INDEX IF NOT EXISTS "idxNotificationDeliveryDiagnosticAuditDeliveryTime"
    ON "public"."sysNotificationDeliveryDiagnosticAudit" ("deliveryId", "createTime");
CREATE INDEX IF NOT EXISTS "idxNotificationDeliveryDiagnosticAuditActorTime"
    ON "public"."sysNotificationDeliveryDiagnosticAudit" ("actorId", "createTime");

-- +goose StatementBegin
DO $$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_permission (
      id, code, name, "resourceType", method, path, status, description,
      "creatorId", "createTime", "updaterId", "updateTime", "isDeleted"
    ) VALUES
      (1900301064, 'system:notification:delivery:diagnostic', '诊断通知投递内容', 'API', 'POST', '/notification/deliveries/:deliveryId/diagnostic-content', 0, '逐条诊断通知投递内容，需原因、受保护连接和按内容级别授权', 0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, FALSE),
      (1900301065, 'system:notification:delivery:content:public', '查看公开通知内容', 'API', 'POST', '/notification/deliveries/:deliveryId/diagnostic-content', 0, '查看公开级别通知内容', 0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, FALSE),
      (1900301066, 'system:notification:delivery:content:sensitive', '查看敏感通知内容', 'API', 'POST', '/notification/deliveries/:deliveryId/diagnostic-content', 0, '查看敏感级别通知内容，需二次确认', 0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, FALSE),
      (1900301067, 'system:notification:delivery:content:secret-ephemeral', '查看短期秘密通知内容', 'API', 'POST', '/notification/deliveries/:deliveryId/diagnostic-content', 0, '查看未过期短期秘密，需二次确认', 0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, FALSE)
    ON CONFLICT DO NOTHING;
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL
     AND to_regclass('public.sys_role') IS NOT NULL
     AND to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_role_permission (
      id, "roleId", "permissionId", source, "creatorId", "createTime", "updateTime"
    )
    SELECT 190030106400 + r.id + p.id, r.id, p.id, 'DIRECT', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
    FROM sys_role r
    JOIN sys_permission p ON p.id BETWEEN 1900301064 AND 1900301067 AND p."isDeleted" = FALSE
    WHERE r."systemKey" = 'AUTHORIZATION_ROOT' AND r."isDeleted" = FALSE
      AND NOT EXISTS (
        SELECT 1 FROM sys_role_permission existing
        WHERE existing."roleId" = r.id AND existing."permissionId" = p.id
      )
    ON CONFLICT DO NOTHING;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Forward-only: audit history and encrypted TTL envelopes must remain for a
-- later incident review. Permission revocation is handled deliberately.
-- +goose StatementBegin
DO $$
BEGIN
  RAISE EXCEPTION 'forward-only notification delivery diagnostics migration cannot be rolled back automatically';
END
$$;
-- +goose StatementEnd
