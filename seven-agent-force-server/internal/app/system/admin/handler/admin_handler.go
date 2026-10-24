package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	stdjson "encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	dockerinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/docker"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/adaptor"
	"github.com/xuri/excelize/v2"
)

type LoginFailureService interface {
	RecordFailure(ctx context.Context, userAccount, clientIP, deviceID string) error
	ClearFailure(ctx context.Context, userAccount string) error
	NeedCaptcha(ctx context.Context, userAccount string) (bool, error)
	IsAccountLocked(ctx context.Context, userAccount string) (bool, error)
	GetUnlockTime(ctx context.Context, userAccount string) (*int64, error)
	GetFailureCount(ctx context.Context, userAccount string) (int, error)
	GetRiskFailureCount(ctx context.Context, userAccount, clientIP, deviceID string) (int, error)
	UnlockAccount(ctx context.Context, userAccount string) error
	LockAccount(ctx context.Context, userAccount string, lockHours int) error
	RecordCaptchaFailure(ctx context.Context, userAccount, clientIP string) error
	ClearCaptchaFailure(ctx context.Context, userAccount string) error
	GetCaptchaFailureCount(ctx context.Context, userAccount string) (int, error)
}

type OnlineUserService interface {
	GetOnlineUsers(ctx context.Context, current, size int64, username, loginIP, browser, os, currentSessionID string) (*adminfacade.PageResult[adminfacade.OnlineUserVO], error)
	GetOnlineUserStats(ctx context.Context) (*adminfacade.OnlineUserStatsVO, error)
	GetUserSession(ctx context.Context, userID int64, currentSessionID string) (*adminfacade.OnlineUserVO, error)
	ForceLogout(ctx context.Context, command adminfacade.ForceLogoutCommand) (bool, error)
	BatchForceLogout(ctx context.Context, command adminfacade.BatchForceLogoutCommand) (*adminfacade.BatchLogoutResultVO, error)
	GetOnlineUserCount(ctx context.Context) (int64, error)
	IsUserOnline(ctx context.Context, userID int64) (bool, error)
}

type OperationLogService interface {
	SaveLogAsync(ctx context.Context, entry adminfacade.OperationLogEntry)
	SaveLog(ctx context.Context, entry adminfacade.OperationLogEntry) error
	GetOperationLogs(ctx context.Context, request adminfacade.OperationLogQueryRequest) (*adminfacade.PageResult[adminfacade.OperationLogVO], error)
	GetOperationLogByID(ctx context.Context, id int64) (*adminfacade.OperationLogVO, error)
	CleanExpiredLogs(ctx context.Context, days int) (int64, error)
	ExportOperationLogs(ctx context.Context, request adminfacade.OperationLogExportRequest, currentUserID int64) ([]adminfacade.OperationLogExportDTO, error)
	DeleteLogsByTimeRange(ctx context.Context, startTime, endTime string) (int64, error)
	GetMyOperationLogs(ctx context.Context, currentUserID int64, request adminfacade.MyOperationLogQueryRequest) (*adminfacade.PageResult[adminfacade.OperationLogVO], error)
	GetOperationTypes(ctx context.Context) []adminfacade.OperationTypeOption
}

type RuntimeLogService interface {
	Page(ctx context.Context, request adminfacade.RuntimeLogQueryDTO) (*adminfacade.PageResult[adminfacade.RuntimeLogLineDTO], error)
	Stream(ctx context.Context, request adminfacade.RuntimeLogStreamRequestDTO, userID int64) (io.ReadCloser, error)
}

type Handler struct {
	loginFailure LoginFailureService
	online       OnlineUserService
	operation    OperationLogService
	runtime      RuntimeLogService
	docker       dockerinfra.Service
	auth         authorizationfacade.AuthFacade
}

func NewHandler(loginFailure LoginFailureService, online OnlineUserService, operation OperationLogService, runtime RuntimeLogService, docker dockerinfra.Service) *Handler {
	return &Handler{
		loginFailure: loginFailure,
		online:       online,
		operation:    operation,
		runtime:      runtime,
		docker:       docker,
	}
}

func (c *Handler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if c == nil {
		return
	}
	c.auth = auth
}

func (c *Handler) GetOnlineUsers(ctx context.Context, reqCtx *app.RequestContext) {
	current := parseQueryInt64WithDefault(reqCtx, "current", 1)
	size := parseQueryInt64WithDefault(reqCtx, "size", 10)
	page, err := c.online.GetOnlineUsers(
		ctx,
		current,
		size,
		strings.TrimSpace(string(reqCtx.Query("userName"))),
		strings.TrimSpace(string(reqCtx.Query("loginIp"))),
		strings.TrimSpace(string(reqCtx.Query("browser"))),
		strings.TrimSpace(string(reqCtx.Query("os"))),
		securitycontext.Require(reqCtx).SessionID,
	)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) GetOnlineUserCount(ctx context.Context, reqCtx *app.RequestContext) {
	count, err := c.online.GetOnlineUserCount(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, count)
}

func (c *Handler) GetOnlineUserStats(ctx context.Context, reqCtx *app.RequestContext) {
	stats, err := c.online.GetOnlineUserStats(ctx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, stats)
}

func (c *Handler) GetUserSession(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parsePathInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, getErr := c.online.GetUserSession(ctx, userID, securitycontext.Require(reqCtx).SessionID)
	if getErr != nil {
		response.Error(reqCtx, getErr)
		return
	}
	if item == nil {
		response.Error(reqCtx, apperrors.NotFound("用户未在线或会话已过期"))
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) KickUser(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parsePathInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := validateKickTarget(securitycontext.Require(reqCtx).UserID, userID); err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := forceLogoutBinding(userID)
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionAdminForceLogout), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	ok, kickErr := c.online.ForceLogout(ctx, adminfacade.ForceLogoutCommand{UserID: userID, OperatorID: securitycontext.Require(reqCtx).UserID, StepUpProof: proof})
	if kickErr != nil {
		response.Error(reqCtx, kickErr)
		return
	}
	response.Success(reqCtx, ok)
}

func (c *Handler) BatchKickUsers(ctx context.Context, reqCtx *app.RequestContext) {
	userIDs, err := parseBatchUserIDs(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	currentUserID := securitycontext.Require(reqCtx).UserID
	for _, userID := range userIDs {
		if err := validateKickTarget(currentUserID, userID); err != nil {
			response.Error(reqCtx, err)
			return
		}
	}
	binding := batchForceLogoutBinding(userIDs)
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionAdminForceLogout), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.online.BatchForceLogout(ctx, adminfacade.BatchForceLogoutCommand{UserIDs: userIDs, OperatorID: currentUserID, StepUpProof: proof})
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) CheckUserOnline(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := parsePathInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	ok, checkErr := c.online.IsUserOnline(ctx, userID)
	if checkErr != nil {
		response.Error(reqCtx, checkErr)
		return
	}
	response.Success(reqCtx, ok)
}

func (c *Handler) GetOperationLogs(ctx context.Context, reqCtx *app.RequestContext) {
	var request adminfacade.OperationLogQueryRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := c.operation.GetOperationLogs(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) GetOperationLogByID(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, getErr := c.operation.GetOperationLogByID(ctx, id)
	if getErr != nil {
		response.Error(reqCtx, getErr)
		return
	}
	if item == nil {
		response.Error(reqCtx, apperrors.NotFound("操作日志不存在"))
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) CleanExpiredLogs(ctx context.Context, reqCtx *app.RequestContext) {
	days := int(parseQueryInt64WithDefault(reqCtx, "days", 30))
	count, err := c.operation.CleanExpiredLogs(ctx, days)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, count)
}

func (c *Handler) GetOperationTypes(ctx context.Context, reqCtx *app.RequestContext) {
	response.Success(reqCtx, c.operation.GetOperationTypes(ctx))
}

func (c *Handler) ExportOperationLogs(ctx context.Context, reqCtx *app.RequestContext) {
	var request adminfacade.OperationLogExportRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	currentUserID, _ := securitycontext.CurrentUserID(reqCtx)
	rows, err := c.operation.ExportOperationLogs(ctx, request, currentUserID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	file, buffer, err := buildOperationLogWorkbook(rows)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	defer file.Close()
	reqCtx.Response.Header.Set("Content-Type", "application/octet-stream")
	reqCtx.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", buildExportFileName()))
	reqCtx.SetStatusCode(http.StatusOK)
	reqCtx.Write(buffer.Bytes()) //nolint:errcheck
}

func (c *Handler) DeleteLogsByTimeRange(ctx context.Context, reqCtx *app.RequestContext) {
	startTime := strings.TrimSpace(string(reqCtx.Query("startTime")))
	endTime := strings.TrimSpace(string(reqCtx.Query("endTime")))
	count, err := c.operation.DeleteLogsByTimeRange(ctx, startTime, endTime)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, count)
}

func (c *Handler) GetMyOperationLogs(ctx context.Context, reqCtx *app.RequestContext) {
	currentUserID, ok := securitycontext.CurrentUserID(reqCtx)
	if !ok {
		response.Error(reqCtx, apperrors.Unauthorized("未登录"))
		return
	}
	var request adminfacade.MyOperationLogQueryRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := c.operation.GetMyOperationLogs(ctx, currentUserID, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) RuntimeLogPage(ctx context.Context, reqCtx *app.RequestContext) {
	var request adminfacade.RuntimeLogQueryDTO
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := c.runtime.Page(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) RuntimeLogStream(ctx context.Context, reqCtx *app.RequestContext) {
	currentUserID, ok := securitycontext.CurrentUserID(reqCtx)
	if !ok {
		response.Error(reqCtx, apperrors.Unauthorized("未登录"))
		return
	}
	var request adminfacade.RuntimeLogStreamRequestDTO
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	stream, err := c.runtime.Stream(ctx, request, currentUserID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	adaptor.HertzHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer stream.Close()
		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("Connection", "keep-alive")
		writer.WriteHeader(http.StatusOK)
		flusher.Flush()

		buffer := make([]byte, 4096)
		for {
			n, readErr := stream.Read(buffer)
			if n > 0 {
				if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
					return
				}
				flusher.Flush()
			}
			if readErr != nil {
				return
			}
		}
	}))(ctx, reqCtx)
}

func parsePathInt64(reqCtx *app.RequestContext, key string) (int64, error) {
	return parseStringInt64(string(reqCtx.Param(key)))
}

func parseStringInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, apperrors.Params("参数错误")
	}
	return parsed, nil
}

func parseQueryInt64WithDefault(reqCtx *app.RequestContext, key string, fallback int64) int64 {
	if reqCtx == nil {
		return fallback
	}
	value := strings.TrimSpace(string(reqCtx.Query(key)))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseBatchUserIDs(reqCtx *app.RequestContext) ([]int64, error) {
	body := bytes.TrimSpace(reqCtx.Request.Body())
	if len(body) == 0 {
		return nil, apperrors.Params("参数错误")
	}
	var userIDs []any
	if err := decodeJSONUseNumber(body, &userIDs); err == nil && len(userIDs) > 0 {
		return sanitizeFlexibleUserIDs(userIDs)
	}
	var payload struct {
		UserIDs []any `json:"userIds"`
	}
	if err := decodeJSONUseNumber(body, &payload); err != nil {
		return nil, apperrors.Params("参数错误")
	}
	return sanitizeFlexibleUserIDs(payload.UserIDs)
}

func decodeJSONUseNumber(body []byte, target any) error {
	decoder := stdjson.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func sanitizeFlexibleUserIDs(userIDs []any) ([]int64, error) {
	if len(userIDs) == 0 {
		return nil, apperrors.Params("参数错误")
	}
	result := make([]int64, 0, len(userIDs))
	for _, rawUserID := range userIDs {
		userID, err := parseFlexibleUserID(rawUserID)
		if err != nil {
			return nil, err
		}
		result = append(result, userID)
	}
	return sanitizeUserIDs(result)
}

func parseFlexibleUserID(raw any) (int64, error) {
	switch value := raw.(type) {
	case stdjson.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, apperrors.Params("参数错误")
		}
		return parsed, nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0, apperrors.Params("参数错误")
		}
		return parsed, nil
	case float64:
		if value != float64(int64(value)) {
			return 0, apperrors.Params("参数错误")
		}
		return int64(value), nil
	default:
		return 0, apperrors.Params("参数错误")
	}
}

func sanitizeUserIDs(userIDs []int64) ([]int64, error) {
	if len(userIDs) == 0 {
		return nil, apperrors.Params("参数错误")
	}
	result := make([]int64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			return nil, apperrors.Params("参数错误")
		}
		result = append(result, userID)
	}
	return result, nil
}

func validateKickTarget(currentUserID, targetUserID int64) error {
	if currentUserID > 0 && targetUserID > 0 && currentUserID == targetUserID {
		return apperrors.Params("不能踢掉自己，请使用其他管理员账号操作")
	}
	return nil
}

func (c *Handler) ensureProtectedMutation(ctx context.Context, reqCtx *app.RequestContext, businessAction, operationBinding string) (stepup.ProofMetadata, error) {
	if c.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	scope, err := buildRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := chooseFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessAction)
	if proofToken != "" {
		token, err := c.auth.VerifyStepUp(ctx, scope, authorizationfacade.StepUpVerifyRequest{
			ProofToken:       proofToken,
			BusinessAction:   businessAction,
			FlowNonce:        flowNonce,
			OperationBinding: operationBinding,
			ConsumeOnce:      true,
		})
		if err != nil {
			return stepup.ProofMetadata{}, err
		}
		if token == nil {
			return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof验证失败")
		}
		securitycontext.SetStepUpProofAudit(reqCtx, stepUpProofAuditFromToken(token, businessAction, operationBinding))
		return stepUpProofMetadataFromToken(token, businessAction, operationBinding), nil
	}
	challenge, err := c.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{
		BusinessAction:   businessAction,
		FlowNonce:        flowNonce,
		OperationBinding: operationBinding,
	})
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	return stepup.ProofMetadata{}, apperrors.ChallengeRequired("", map[string]any{
		"challengeIdentifier":        challenge.ChallengeIdentifier,
		"challengeState":             challenge.ChallengeState,
		"effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds,
		"requiredAssuranceLevel":     challenge.RequiredAssuranceLevel,
		"resolvedAssuranceLevel":     challenge.ResolvedAssuranceLevel,
		"recommendedStepIdentifier":  challenge.RecommendedStepIdentifier,
		"actualChallengeTypeNames":   challenge.ActualChallengeTypeNames,
		"flowNonce":                  flowNonce,
		"steps":                      challenge.Steps,
		"operationBinding":           operationBinding,
	})
}

func buildRequestScope(reqCtx *app.RequestContext) (authorizationfacade.RequestScope, error) {
	user := securitycontext.Require(reqCtx)
	if user.UserID <= 0 {
		return authorizationfacade.RequestScope{}, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return authorizationfacade.RequestScope{
		UserID:    user.UserID,
		Username:  user.Username,
		IPAddress: reqCtx.ClientIP(),
		UserAgent: string(reqCtx.UserAgent()),
		DeviceID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Device-Id"))),
		TenantID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Tenant-Id"))),
		SessionID: user.SessionID,
		Source:    user.Source,
	}, nil
}

func stepUpProofAuditFromToken(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) securitycontext.StepUpProofAudit {
	if token == nil {
		return securitycontext.StepUpProofAudit{}
	}
	return securitycontext.StepUpProofAudit{
		BusinessAction:        firstNonBlank(token.BusinessAction, businessAction),
		OperationBinding:      firstNonBlank(token.OperationBinding, operationBinding),
		ProofIdentifier:       token.TokenUniqueIdentifier,
		ChallengeIdentifier:   token.ChallengeID,
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...),
	}
}

func stepUpProofMetadataFromToken(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) stepup.ProofMetadata {
	if token == nil {
		return stepup.ProofMetadata{}
	}
	return stepup.ProofMetadata{
		BusinessAction:        firstNonBlank(token.BusinessAction, businessAction),
		OperationBinding:      firstNonBlank(token.OperationBinding, operationBinding),
		ProofIdentifier:       token.TokenUniqueIdentifier,
		ChallengeIdentifier:   token.ChallengeID,
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...),
	}
}

func firstNonBlank(values ...string) string {
	for _, item := range values {
		if value := strings.TrimSpace(item); value != "" {
			return value
		}
	}
	return ""
}

func chooseFlowNonce(flowNonce, businessAction string) string {
	value := strings.TrimSpace(flowNonce)
	if value == "" {
		return strings.ToLower(strings.TrimSpace(businessAction)) + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	return value
}

func forceLogoutBinding(userID int64) string {
	return "user:" + strconv.FormatInt(userID, 10) + "|force-logout"
}

const maxInlineBatchForceLogoutBindingLength = 512

func batchForceLogoutBinding(userIDs []int64) string {
	joined := joinSortedUserIDs(userIDs)
	inline := "users:" + joined + "|force-logout"
	if len(inline) <= maxInlineBatchForceLogoutBindingLength {
		return inline
	}
	sum := sha256.Sum256([]byte(joined))
	return "users:count=" + strconv.Itoa(len(sanitizePositiveUserIDs(userIDs))) + ",sha256=" + hex.EncodeToString(sum[:]) + "|force-logout"
}

func joinSortedUserIDs(ids []int64) string {
	normalized := sanitizePositiveUserIDs(ids)
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	parts := make([]string, 0, len(normalized))
	for _, id := range normalized {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func sanitizePositiveUserIDs(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func buildOperationLogWorkbook(rows []adminfacade.OperationLogExportDTO) (*excelize.File, *bytes.Buffer, error) {
	file := excelize.NewFile()
	sheet := file.GetSheetName(0)
	headers := []string{
		"ID", "操作时间", "用户名", "昵称", "操作类型", "操作名称", "操作描述", "方法名",
		"请求方式", "请求地址", "TraceID", "请求IP", "请求地区", "浏览器", "操作系统", "执行耗时(ms)", "状态", "错误信息", "创建时间",
	}
	for idx, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(idx+1, 1)
		file.SetCellValue(sheet, cell, header) //nolint:errcheck
	}
	for rowIdx, item := range rows {
		values := []any{
			item.ID,
			formatTime(item.OperationTime),
			item.UserName,
			item.NickName,
			item.OperationType,
			item.OperationTypeLabel,
			item.OperationDesc,
			item.MethodName,
			item.RequestMethod,
			item.RequestURL,
			item.TraceID,
			item.RequestIP,
			item.RequestLocation,
			item.Browser,
			item.OS,
			item.ExecutionTime,
			item.Status,
			item.ErrorMsg,
			formatTime(item.CreateTime),
		}
		for colIdx, value := range values {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			file.SetCellValue(sheet, cell, value) //nolint:errcheck
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, nil, err
	}
	return file, buffer, nil
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04:05")
}

func buildExportFileName() string {
	return "operation_logs_" + time.Now().UTC().Format("2006-01-02_15-04-05") + ".xlsx"
}
