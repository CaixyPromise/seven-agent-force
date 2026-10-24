package application

import (
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func mapSceneRevision(item domain.SceneRevision, receiverKind string) *facade.SceneRevisionRecord {
	return &facade.SceneRevisionRecord{
		ID:                 item.ID,
		RevisionNo:         item.RevisionNo,
		State:              item.State,
		RevisionVersion:    item.RevisionVersion,
		Enabled:            item.Enabled,
		TemplateRevisionID: item.TemplateRevisionID,
		ConnectionRef:      item.ConnectionRef,
		SendingWay:         sceneSendingWay(receiverKind, item.ConnectionRef),
		PublishedAt:        item.PublishedAt,
		PublishedBy:        item.PublishedBy,
		CreateTime:         item.CreateTime,
		UpdateTime:         item.UpdateTime,
	}
}

func mapSceneDefinition(item domain.SceneDefinition, draft, published *domain.SceneRevision) *facade.SceneDefinitionRecord {
	record := &facade.SceneDefinitionRecord{
		ID:           item.ID,
		SceneCode:    item.SceneCode,
		SceneName:    item.SceneName,
		ReceiverKind: item.ReceiverKind,
		Version:      item.Version,
		CreateTime:   item.CreateTime,
		UpdateTime:   item.UpdateTime,
	}
	if draft != nil {
		record.CurrentDraft = mapSceneRevision(*draft, item.ReceiverKind)
	}
	if published != nil {
		record.CurrentPublished = mapSceneRevision(*published, item.ReceiverKind)
	}
	return record
}

func mapSceneRevisions(items []domain.SceneRevision, receiverKind string) []facade.SceneRevisionRecord {
	if len(items) == 0 {
		return nil
	}
	result := make([]facade.SceneRevisionRecord, 0, len(items))
	for _, item := range items {
		result = append(result, *mapSceneRevision(item, receiverKind))
	}
	return result
}

func sceneSendingWay(receiverKind, connectionRef string) string {
	switch strings.ToUpper(strings.TrimSpace(receiverKind)) {
	case domain.SceneReceiverKindInApp:
		return "站内信"
	case domain.SceneReceiverKindFeishuOpenID:
		return "飞书应用 · 指定成员"
	case domain.SceneReceiverKindFeishuChatID:
		return "飞书应用 · 指定群聊"
	case domain.SceneReceiverKindWeComUserID:
		return "企业微信应用 · 指定成员"
	case domain.SceneReceiverKindFixedConnection:
		if strings.TrimSpace(connectionRef) == "" {
			return "受控连接"
		}
		return "受控连接 · " + strings.TrimSpace(connectionRef)
	default:
		return ""
	}
}
