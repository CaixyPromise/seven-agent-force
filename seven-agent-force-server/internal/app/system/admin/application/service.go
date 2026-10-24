package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

const stepUpActionAdminForceLogout = "ADMIN_FORCE_LOGOUT"

type runtimeLogProvider interface {
	Page(ctx context.Context, query domain.RuntimeLogPageQuery) ([]domain.RuntimeLogLine, int64, error)
	Stream(ctx context.Context, query domain.RuntimeLogStreamQuery, userID int64) (io.ReadCloser, error)
}

type asyncOperationLogWriter interface {
	Enqueue(ctx context.Context, item domain.OperationLog)
}

type Service struct {
	loginCfg      LoginSettings
	subjects      userfacade.SubjectFacade
	accounts      userfacade.AccountFacade
	auth          authorizationfacade.AuthFacade
	sessions      ssofacade.SessionFacade
	onlineStore   domain.OnlineUserStateStore
	loginFailures domain.LoginFailureStateStore
	oplogRepo     domain.OperationLogRepository
	oplogWriter   asyncOperationLogWriter
	runtimeLogs   runtimeLogProvider
	domain        *domain.Service
}

type LoginSettings struct {
	CaptchaThreshold     int
	LockThreshold        int
	ContextLockThreshold int
	LockDurationHours    int
}

func NewService(
	loginCfg LoginSettings,
	subjects userfacade.SubjectFacade,
	accounts userfacade.AccountFacade,
	auth authorizationfacade.AuthFacade,
	sessions ssofacade.SessionFacade,
	onlineStore domain.OnlineUserStateStore,
	loginFailures domain.LoginFailureStateStore,
	oplogRepo domain.OperationLogRepository,
	oplogWriter asyncOperationLogWriter,
	runtimeLogs runtimeLogProvider,
	domainService *domain.Service,
) *Service {
	return &Service{
		loginCfg:      loginCfg,
		subjects:      subjects,
		accounts:      accounts,
		auth:          auth,
		sessions:      sessions,
		onlineStore:   onlineStore,
		loginFailures: loginFailures,
		oplogRepo:     oplogRepo,
		oplogWriter:   oplogWriter,
		runtimeLogs:   runtimeLogs,
		domain:        domainService,
	}
}

func (s *Service) RecordFailure(ctx context.Context, userAccount, clientIP, deviceID string) error {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	count, err := s.loginFailures.GetFailureCount(ctx, userAccount)
	if err != nil {
		return err
	}
	count++
	if err := s.loginFailures.SaveFailureCount(ctx, userAccount, count); err != nil {
		return err
	}
	contextCount, err := s.recordContextFailures(ctx, clientIP, deviceID)
	if err != nil {
		return err
	}
	if count >= s.lockThreshold() || contextCount >= s.contextLockThreshold() {
		unlockAt := time.Now().UTC().Add(time.Duration(s.lockDurationHours()) * time.Hour)
		if err := s.loginFailures.SaveLockUntil(ctx, userAccount, unlockAt.UnixMilli(), s.lockDurationHours()); err != nil {
			return err
		}
		if err := s.syncUserLockState(ctx, userAccount, 1, &unlockAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) GetRiskFailureCount(ctx context.Context, userAccount, clientIP, deviceID string) (int, error) {
	accountCount, err := s.GetFailureCount(ctx, userAccount)
	if err != nil {
		return 0, err
	}
	contextCount, err := s.maxContextFailureCount(ctx, clientIP, deviceID)
	if err != nil {
		return 0, err
	}
	if contextCount > accountCount {
		return contextCount, nil
	}
	return accountCount, nil
}

func (s *Service) ClearFailure(ctx context.Context, userAccount string) error {
	if strings.TrimSpace(userAccount) == "" {
		return nil
	}
	if s.loginFailures != nil {
		if err := s.loginFailures.DeleteFailureCount(ctx, userAccount); err != nil {
			return err
		}
		if err := s.loginFailures.DeleteLock(ctx, userAccount); err != nil {
			return err
		}
		if err := s.loginFailures.DeleteCaptchaFailureCount(ctx, userAccount); err != nil {
			return err
		}
	}
	return s.syncUserLockState(ctx, userAccount, 0, nil)
}

func (s *Service) NeedCaptcha(ctx context.Context, userAccount string) (bool, error) {
	count, err := s.GetFailureCount(ctx, userAccount)
	if err != nil {
		return false, err
	}
	return count >= s.captchaThreshold(), nil
}

func (s *Service) IsAccountLocked(ctx context.Context, userAccount string) (bool, error) {
	unlockTime, err := s.GetUnlockTime(ctx, userAccount)
	if err != nil {
		return false, err
	}
	if unlockTime != nil {
		return true, nil
	}
	subject, err := s.findSubject(ctx, userAccount)
	if err != nil {
		return false, err
	}
	return subject != nil && subject.LockStatus, nil
}

func (s *Service) GetUnlockTime(ctx context.Context, userAccount string) (*int64, error) {
	if s.loginFailures != nil {
		unlockTime, err := s.loginFailures.GetLockUntil(ctx, userAccount)
		if err != nil {
			return nil, err
		}
		if unlockTime != nil {
			if *unlockTime > 0 && *unlockTime <= time.Now().UTC().UnixMilli() {
				if err := s.ClearFailure(ctx, userAccount); err != nil {
					return nil, err
				}
				return nil, nil
			}
			return unlockTime, nil
		}
	}
	subject, err := s.findSubject(ctx, userAccount)
	if err != nil {
		return nil, err
	}
	if subject == nil || !subject.LockStatus {
		return nil, nil
	}
	if subject.UnsealAt == nil {
		return nil, nil
	}
	unlockAt := subject.UnsealAt.UTC().UnixMilli()
	if unlockAt <= time.Now().UTC().UnixMilli() {
		if err := s.ClearFailure(ctx, userAccount); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if s.loginFailures != nil {
		ttlHours := int(time.Until(subject.UnsealAt.UTC()).Hours())
		if ttlHours <= 0 {
			ttlHours = 1
		}
		_ = s.loginFailures.SaveLockUntil(ctx, userAccount, unlockAt, ttlHours)
	}
	return &unlockAt, nil
}

func (s *Service) GetFailureCount(ctx context.Context, userAccount string) (int, error) {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return 0, nil
	}
	return s.loginFailures.GetFailureCount(ctx, userAccount)
}

func (s *Service) UnlockAccount(ctx context.Context, userAccount string) error {
	return s.ClearFailure(ctx, userAccount)
}

func (s *Service) LockAccount(ctx context.Context, userAccount string, lockHours int) error {
	if strings.TrimSpace(userAccount) == "" {
		return nil
	}
	if lockHours <= 0 {
		return s.ClearFailure(ctx, userAccount)
	}
	var unsealAt *time.Time
	value := time.Now().UTC().Add(time.Duration(lockHours) * time.Hour)
	unsealAt = &value
	if s.loginFailures != nil {
		if err := s.loginFailures.SaveLockUntil(ctx, userAccount, value.UnixMilli(), lockHours); err != nil {
			return err
		}
	}
	return s.syncUserLockState(ctx, userAccount, 1, unsealAt)
}

func (s *Service) RecordCaptchaFailure(ctx context.Context, userAccount, clientIP string) error {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	count, err := s.loginFailures.GetCaptchaFailureCount(ctx, userAccount)
	if err != nil {
		return err
	}
	return s.loginFailures.SaveCaptchaFailureCount(ctx, userAccount, count+1)
}

func (s *Service) recordContextFailures(ctx context.Context, clientIP, deviceID string) (int, error) {
	maxCount := 0
	for _, signal := range loginContextSignals(clientIP, deviceID) {
		count, err := s.loginFailures.GetContextFailureCount(ctx, signal.scope, signal.value)
		if err != nil {
			return 0, err
		}
		count++
		if err := s.loginFailures.SaveContextFailureCount(ctx, signal.scope, signal.value, count); err != nil {
			return 0, err
		}
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount, nil
}

func (s *Service) maxContextFailureCount(ctx context.Context, clientIP, deviceID string) (int, error) {
	if s.loginFailures == nil {
		return 0, nil
	}
	maxCount := 0
	for _, signal := range loginContextSignals(clientIP, deviceID) {
		count, err := s.loginFailures.GetContextFailureCount(ctx, signal.scope, signal.value)
		if err != nil {
			return 0, err
		}
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount, nil
}

type loginContextSignal struct {
	scope string
	value string
}

func loginContextSignals(clientIP, deviceID string) []loginContextSignal {
	clientIP = strings.TrimSpace(clientIP)
	deviceID = strings.TrimSpace(deviceID)
	signals := make([]loginContextSignal, 0, 3)
	if clientIP != "" {
		signals = append(signals, loginContextSignal{scope: "ip", value: clientIP})
	}
	if deviceID != "" {
		signals = append(signals, loginContextSignal{scope: "device", value: deviceID})
	}
	if clientIP != "" && deviceID != "" {
		signals = append(signals, loginContextSignal{scope: "ip_device", value: clientIP + "|" + deviceID})
	}
	return signals
}

func (s *Service) ClearCaptchaFailure(ctx context.Context, userAccount string) error {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	return s.loginFailures.DeleteCaptchaFailureCount(ctx, userAccount)
}

func (s *Service) GetCaptchaFailureCount(ctx context.Context, userAccount string) (int, error) {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return 0, nil
	}
	return s.loginFailures.GetCaptchaFailureCount(ctx, userAccount)
}

func (s *Service) GetOnlineUsers(ctx context.Context, current, size int64, username, loginIP, browser, os, currentSessionID string) (*adminfacade.PageResult[adminfacade.OnlineUserVO], error) {
	items, err := s.listOnlineUsers(ctx, currentSessionID)
	if err != nil {
		return nil, err
	}
	filtered := s.domain.FilterOnlineUsers(items, username, loginIP, browser, os)
	pageItems, total := s.domain.PaginateOnlineUsers(filtered, current, size)
	records := make([]adminfacade.OnlineUserVO, 0, len(pageItems))
	for _, item := range pageItems {
		records = append(records, toOnlineUserVO(item))
	}
	return &adminfacade.PageResult[adminfacade.OnlineUserVO]{
		Current: normalizePage(current, 1),
		Size:    normalizePage(size, 10),
		Total:   total,
		Records: records,
	}, nil
}

func (s *Service) GetOnlineUserStats(ctx context.Context) (*adminfacade.OnlineUserStatsVO, error) {
	items, err := s.listOnlineUsers(ctx, "")
	if err != nil {
		return nil, err
	}
	stats := s.domain.BuildOnlineUserStats(items)
	return &adminfacade.OnlineUserStatsVO{
		TotalOnlineUsers: stats.TotalOnlineUsers,
		AdminUsers:       stats.AdminUsers,
		NormalUsers:      stats.NormalUsers,
		BrowserStats:     stats.BrowserStats,
		OSStats:          stats.OSStats,
		TodayLoginUsers:  stats.TodayLoginUsers,
		ActiveUsers:      stats.ActiveUsers,
		TotalOnline:      stats.TotalOnline,
		TodayLogin:       stats.TodayLogin,
		PeakOnline:       stats.PeakOnline,
	}, nil
}

func (s *Service) GetUserSession(ctx context.Context, userID int64, currentSessionID string) (*adminfacade.OnlineUserVO, error) {
	if userID <= 0 {
		return nil, apperrors.Params("用户ID无效")
	}
	if s.sessions == nil {
		return nil, apperrors.Operation("在线用户能力未配置")
	}
	sessions, err := s.sessions.ListSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	item, ok := s.pickLatestSession(ctx, sessions, currentSessionID)
	if !ok {
		if s.onlineStore != nil {
			_ = s.onlineStore.Delete(ctx, userID)
		}
		return nil, nil
	}
	if s.onlineStore != nil {
		_ = s.onlineStore.Save(ctx, userID, &item)
	}
	value := toOnlineUserVO(item)
	return &value, nil
}

func (s *Service) ForceLogout(ctx context.Context, command adminfacade.ForceLogoutCommand) (bool, error) {
	userID := command.UserID
	if userID <= 0 {
		return false, apperrors.Params("用户ID无效")
	}
	if command.OperatorID > 0 && command.OperatorID == userID {
		return false, apperrors.Params("不能踢掉自己，请使用其他管理员账号操作")
	}
	if err := stepup.Require(command.StepUpProof, stepUpActionAdminForceLogout, forceLogoutBinding(userID)); err != nil {
		return false, err
	}
	return s.forceLogout(ctx, userID)
}

func (s *Service) forceLogout(ctx context.Context, userID int64) (bool, error) {
	if s.sessions == nil {
		return false, apperrors.Operation("在线用户能力未配置")
	}
	if _, err := s.sessions.RevokeSessionsByUserID(ctx, userID); err != nil {
		return false, err
	}
	if s.onlineStore != nil {
		_ = s.onlineStore.Delete(ctx, userID)
	}
	return true, nil
}

func (s *Service) BatchForceLogout(ctx context.Context, command adminfacade.BatchForceLogoutCommand) (*adminfacade.BatchLogoutResultVO, error) {
	userIDs := sanitizePositiveUserIDs(command.UserIDs)
	if len(userIDs) == 0 {
		return nil, apperrors.Params("用户ID不能为空")
	}
	if command.OperatorID > 0 {
		for _, userID := range userIDs {
			if command.OperatorID == userID {
				return nil, apperrors.Params("不能踢掉自己，请使用其他管理员账号操作")
			}
		}
	}
	if err := stepup.Require(command.StepUpProof, stepUpActionAdminForceLogout, batchForceLogoutBinding(userIDs)); err != nil {
		return nil, err
	}
	result := &adminfacade.BatchLogoutResultVO{
		SuccessIDs:     []int64{},
		FailedIDs:      []int64{},
		FailureReasons: []string{},
		TotalCount:     len(userIDs),
	}
	for _, userID := range userIDs {
		ok, err := s.forceLogout(ctx, userID)
		if err != nil || !ok {
			result.FailedIDs = append(result.FailedIDs, userID)
			reason := fmt.Sprintf("用户ID %d 下线失败", userID)
			if err != nil {
				reason = fmt.Sprintf("用户ID %d 下线失败: %s", userID, err.Error())
			}
			result.FailureReasons = append(result.FailureReasons, reason)
			continue
		}
		result.SuccessIDs = append(result.SuccessIDs, userID)
	}
	result.SuccessCount = len(result.SuccessIDs)
	result.FailedCount = len(result.FailedIDs)
	return result, nil
}

func (s *Service) GetOnlineUserCount(ctx context.Context) (int64, error) {
	if s.sessions == nil {
		return 0, apperrors.Operation("在线用户能力未配置")
	}
	return s.sessions.CountActiveSessions(ctx)
}

func (s *Service) IsUserOnline(ctx context.Context, userID int64) (bool, error) {
	item, err := s.GetUserSession(ctx, userID, "")
	if err != nil {
		return false, err
	}
	return item != nil, nil
}

func (s *Service) SaveLogAsync(ctx context.Context, entry adminfacade.OperationLogEntry) {
	if s.oplogWriter == nil {
		return
	}
	s.oplogWriter.Enqueue(ctx, mapOperationEntry(entry))
}

func (s *Service) SaveLog(ctx context.Context, entry adminfacade.OperationLogEntry) error {
	if s.oplogRepo == nil {
		return nil
	}
	_, err := s.oplogRepo.InsertOperationLog(ctx, ptr(mapOperationEntry(entry)))
	return err
}

func (s *Service) GetOperationLogs(ctx context.Context, request adminfacade.OperationLogQueryRequest) (*adminfacade.PageResult[adminfacade.OperationLogVO], error) {
	if s.oplogRepo == nil {
		return nil, apperrors.Operation("操作日志能力未配置")
	}
	startTime, err := parseTime(request.StartTime)
	if err != nil {
		return nil, err
	}
	endTime, err := parseTime(request.EndTime)
	if err != nil {
		return nil, err
	}
	items, total, err := s.oplogRepo.QueryOperationLogs(ctx, domain.OperationLogPageQuery{
		Current:       normalizePage(request.Current, 1),
		Size:          normalizePage(request.Size, 10),
		OperationType: stringValue(request.OperationType),
		Username:      strings.TrimSpace(request.Username),
		StartTime:     startTime,
		EndTime:       endTime,
	})
	if err != nil {
		return nil, err
	}
	return &adminfacade.PageResult[adminfacade.OperationLogVO]{
		Current: normalizePage(request.Current, 1),
		Size:    normalizePage(request.Size, 10),
		Total:   total,
		Records: toOperationLogVOs(items),
	}, nil
}

func (s *Service) GetOperationLogByID(ctx context.Context, id int64) (*adminfacade.OperationLogVO, error) {
	if s.oplogRepo == nil {
		return nil, apperrors.Operation("操作日志能力未配置")
	}
	item, err := s.oplogRepo.FindOperationLogByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	value := toOperationLogVO(*item)
	return &value, nil
}

func (s *Service) CleanExpiredLogs(ctx context.Context, days int) (int64, error) {
	if s.oplogRepo == nil {
		return 0, apperrors.Operation("操作日志能力未配置")
	}
	return s.oplogRepo.DeleteOperationLogsBeforeDays(ctx, days)
}

func (s *Service) ExportOperationLogs(ctx context.Context, request adminfacade.OperationLogExportRequest, currentUserID int64) ([]adminfacade.OperationLogExportDTO, error) {
	page, err := s.GetOperationLogs(ctx, adminfacade.OperationLogQueryRequest{
		Current:       1,
		Size:          10000,
		OperationType: request.OperationType,
		StartTime:     request.StartTime,
		EndTime:       request.EndTime,
	})
	if err != nil {
		return nil, err
	}
	items := make([]adminfacade.OperationLogExportDTO, 0, len(page.Records))
	for _, item := range page.Records {
		items = append(items, adminfacade.OperationLogExportDTO{
			ID:                 item.ID,
			OperationTime:      item.OperationTime,
			UserName:           item.UserName,
			NickName:           item.NickName,
			OperationType:      item.OperationType,
			OperationTypeDesc:  item.OperationTypeDesc,
			OperationTypeLabel: item.OperationTypeLabel,
			OperationDesc:      item.OperationDesc,
			MethodName:         item.MethodName,
			RequestMethod:      item.RequestMethod,
			RequestURL:         item.RequestURL,
			TraceID:            item.TraceID,
			RequestIP:          item.RequestIP,
			RequestLocation:    item.RequestLocation,
			Browser:            item.Browser,
			OS:                 item.OS,
			ExecutionTime:      item.ExecutionTime,
			Status:             mapStatusText(item.Status),
			ErrorMsg:           item.ErrorMsg,
			CreateTime:         item.CreateTime,
		})
	}
	return items, nil
}

func (s *Service) DeleteLogsByTimeRange(ctx context.Context, startTime, endTime string) (int64, error) {
	if s.oplogRepo == nil {
		return 0, apperrors.Operation("操作日志能力未配置")
	}
	return s.oplogRepo.DeleteOperationLogsByTimeRange(ctx, strings.TrimSpace(startTime), strings.TrimSpace(endTime))
}

func (s *Service) GetMyOperationLogs(ctx context.Context, currentUserID int64, request adminfacade.MyOperationLogQueryRequest) (*adminfacade.PageResult[adminfacade.OperationLogVO], error) {
	if currentUserID <= 0 {
		return nil, apperrors.Unauthorized("未登录")
	}
	startTime, err := parseTime(request.StartTime)
	if err != nil {
		return nil, err
	}
	endTime, err := parseTime(request.EndTime)
	if err != nil {
		return nil, err
	}
	items, total, err := s.oplogRepo.QueryOperationLogs(ctx, domain.OperationLogPageQuery{
		Current:          normalizePage(request.Current, 1),
		Size:             normalizePage(request.Size, 10),
		UserID:           currentUserID,
		OperationType:    strings.TrimSpace(request.OperationType),
		RequestMethod:    strings.TrimSpace(request.RequestMethod),
		RequestURL:       strings.TrimSpace(request.RequestURL),
		ExecutionTimeMin: request.ExecutionTimeMin,
		ExecutionTimeMax: request.ExecutionTimeMax,
		StartTime:        startTime,
		EndTime:          endTime,
	})
	if err != nil {
		return nil, err
	}
	return &adminfacade.PageResult[adminfacade.OperationLogVO]{
		Current: normalizePage(request.Current, 1),
		Size:    normalizePage(request.Size, 10),
		Total:   total,
		Records: toOperationLogVOs(items),
	}, nil
}

func (s *Service) GetOperationTypes(ctx context.Context) []adminfacade.OperationTypeOption {
	return adminfacade.OperationTypeOptions()
}

func (s *Service) Page(ctx context.Context, request adminfacade.RuntimeLogQueryDTO) (*adminfacade.PageResult[adminfacade.RuntimeLogLineDTO], error) {
	if s.runtimeLogs == nil {
		return nil, apperrors.Operation("运行日志能力未配置")
	}
	startTime, err := parseTime(request.StartTime)
	if err != nil {
		return nil, err
	}
	endTime, err := parseTime(request.EndTime)
	if err != nil {
		return nil, err
	}
	lines, total, err := s.runtimeLogs.Page(ctx, domain.RuntimeLogPageQuery{
		Current:        normalizePage(request.Current, 1),
		Size:           normalizePage(request.Size, 20),
		Keyword:        strings.TrimSpace(request.Keyword),
		ContentKeyword: strings.TrimSpace(request.ContentKeyword),
		Level:          strings.TrimSpace(request.Level),
		LoggerName:     strings.TrimSpace(request.LoggerName),
		ThreadName:     strings.TrimSpace(request.ThreadName),
		TraceID:        strings.TrimSpace(request.TraceID),
		StartTime:      startTime,
		EndTime:        endTime,
		UseRegex:       request.UseRegex,
	})
	if err != nil {
		return nil, err
	}
	result := make([]adminfacade.RuntimeLogLineDTO, 0, len(lines))
	for _, item := range lines {
		result = append(result, adminfacade.RuntimeLogLineDTO(item))
	}
	return &adminfacade.PageResult[adminfacade.RuntimeLogLineDTO]{
		Current: normalizePage(request.Current, 1),
		Size:    normalizePage(request.Size, 20),
		Total:   total,
		Records: result,
	}, nil
}

func (s *Service) Stream(ctx context.Context, request adminfacade.RuntimeLogStreamRequestDTO, userID int64) (io.ReadCloser, error) {
	if s.runtimeLogs == nil {
		return nil, apperrors.Operation("运行日志能力未配置")
	}
	return s.runtimeLogs.Stream(ctx, domain.RuntimeLogStreamQuery{
		Keyword:        strings.TrimSpace(request.Keyword),
		ContentKeyword: strings.TrimSpace(request.ContentKeyword),
		Level:          strings.TrimSpace(request.Level),
		LoggerName:     strings.TrimSpace(request.LoggerName),
		ThreadName:     strings.TrimSpace(request.ThreadName),
		TraceID:        strings.TrimSpace(request.TraceID),
		LastN:          request.LastN,
		UseRegex:       request.UseRegex,
	}, userID)
}

func (s *Service) listOnlineUsers(ctx context.Context, currentSessionID string) ([]domain.OnlineUser, error) {
	if s.sessions == nil {
		return nil, apperrors.Operation("在线用户能力未配置")
	}
	sessions, err := s.sessions.ListActiveSessions(ctx)
	if err != nil {
		return nil, err
	}
	grouped := map[int64][]ssofacade.SessionRecord{}
	for _, item := range sessions {
		if item.UserID <= 0 {
			continue
		}
		grouped[item.UserID] = append(grouped[item.UserID], item)
	}
	result := make([]domain.OnlineUser, 0, len(grouped))
	for userID, list := range grouped {
		item, ok := s.pickLatestSession(ctx, list, currentSessionID)
		if !ok {
			continue
		}
		if s.onlineStore != nil {
			_ = s.onlineStore.Save(ctx, userID, &item)
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) pickLatestSession(ctx context.Context, sessions []ssofacade.SessionRecord, currentSessionID string) (domain.OnlineUser, bool) {
	var latest *ssofacade.SessionRecord
	for idx := range sessions {
		item := sessions[idx]
		if latest == nil || compareSessionActivity(item, *latest) > 0 {
			candidate := item
			latest = &candidate
		}
	}
	if latest == nil {
		return domain.OnlineUser{}, false
	}
	return s.buildOnlineUser(ctx, *latest, currentSessionID)
}

func (s *Service) buildOnlineUser(ctx context.Context, session ssofacade.SessionRecord, currentSessionID string) (domain.OnlineUser, bool) {
	user := domain.OnlineUser{
		UserID:           session.UserID,
		LoginTime:        session.LoginAt,
		LastActiveTime:   session.LastAccessAt,
		ExpireTime:       session.ExpiresAt,
		LoginIP:          strings.TrimSpace(session.LoginIP),
		DeviceID:         strings.TrimSpace(session.DeviceID),
		UserAgent:        strings.TrimSpace(session.UserAgent),
		TokenID:          strings.TrimSpace(session.SessionID),
		IsCurrentSession: strings.TrimSpace(currentSessionID) != "" && strings.TrimSpace(currentSessionID) == strings.TrimSpace(session.SessionID),
	}
	user.Browser, user.OS = detectBrowserOS(user.UserAgent)
	if s.auth != nil {
		if info, err := s.auth.GetUserVO(ctx, session.UserID); err == nil && info != nil {
			user.Username = info.Username
			user.Nickname = info.Nickname
			user.Avatar = info.Avatar
			user.Email = info.Email
			user.UserRole = deriveUserRole(info)
		}
	}
	if strings.TrimSpace(user.Username) == "" && s.subjects != nil {
		if subject, err := s.subjects.FindSubjectByID(ctx, session.UserID); err == nil && subject != nil {
			user.Username = subject.AccountName
			user.Email = subject.Email
		}
	}
	if strings.TrimSpace(user.Username) == "" {
		return domain.OnlineUser{}, false
	}
	return user, true
}

func (s *Service) findSubject(ctx context.Context, userAccount string) (*userfacade.SubjectRecord, error) {
	if s.subjects == nil || strings.TrimSpace(userAccount) == "" {
		return nil, nil
	}
	return s.subjects.FindSubjectByAccount(ctx, strings.TrimSpace(userAccount))
}

func (s *Service) syncUserLockState(ctx context.Context, userAccount string, status int, unsealAt *time.Time) error {
	if s.subjects == nil || s.accounts == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	subject, err := s.subjects.FindSubjectByAccount(ctx, strings.TrimSpace(userAccount))
	if err != nil || subject == nil {
		return err
	}
	return s.accounts.UpdateLockState(ctx, userfacade.UpdateLockStateCommand{
		UserID:     subject.UserID,
		Status:     status,
		UnsealTime: unsealAt,
	})
}

func (s *Service) captchaThreshold() int {
	if s.loginCfg.CaptchaThreshold > 0 {
		return s.loginCfg.CaptchaThreshold
	}
	return 3
}

func (s *Service) lockThreshold() int {
	if s.loginCfg.LockThreshold > 0 {
		return s.loginCfg.LockThreshold
	}
	return 10
}

func (s *Service) contextLockThreshold() int {
	if s.loginCfg.ContextLockThreshold > 0 {
		return s.loginCfg.ContextLockThreshold
	}
	return s.lockThreshold()
}

func (s *Service) lockDurationHours() int {
	if s.loginCfg.LockDurationHours > 0 {
		return s.loginCfg.LockDurationHours
	}
	return 24
}

func parseTime(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	candidates := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	raw := strings.TrimSpace(*value)
	for _, layout := range candidates {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			utc := parsed.UTC()
			return &utc, nil
		}
	}
	return nil, apperrors.Params("时间格式非法")
}

func normalizePage(value, fallback int64) int64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func toOnlineUserVO(item domain.OnlineUser) adminfacade.OnlineUserVO {
	return adminfacade.OnlineUserVO{
		UserID:           item.UserID,
		Username:         item.Username,
		Nickname:         item.Nickname,
		Avatar:           item.Avatar,
		Email:            item.Email,
		UserRole:         item.UserRole,
		LoginTime:        toMillis(item.LoginTime),
		LastActiveTime:   toMillis(item.LastActiveTime),
		ExpireTime:       toMillis(item.ExpireTime),
		LoginIP:          item.LoginIP,
		LoginAddress:     item.LoginAddress,
		Browser:          item.Browser,
		OS:               item.OS,
		DeviceID:         item.DeviceID,
		UserAgent:        item.UserAgent,
		TokenID:          item.TokenID,
		IsCurrentSession: item.IsCurrentSession,
	}
}

func toMillis(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	millis := value.UTC().UnixMilli()
	return &millis
}

func mapOperationEntry(entry adminfacade.OperationLogEntry) domain.OperationLog {
	now := entry.OperationTime
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return domain.OperationLog{
		UserID:          entry.UserID,
		UserName:        strings.TrimSpace(entry.UserName),
		NickName:        strings.TrimSpace(entry.NickName),
		OperationType:   strings.TrimSpace(string(entry.OperationType)),
		OperationDesc:   strings.TrimSpace(entry.OperationDesc),
		MethodName:      strings.TrimSpace(entry.MethodName),
		RequestMethod:   strings.TrimSpace(entry.RequestMethod),
		RequestURL:      strings.TrimSpace(entry.RequestURL),
		TraceID:         strings.TrimSpace(entry.TraceID),
		RequestParams:   strings.TrimSpace(entry.RequestParams),
		ResponseResult:  strings.TrimSpace(entry.ResponseResult),
		RequestIP:       strings.TrimSpace(entry.RequestIP),
		RequestLocation: strings.TrimSpace(entry.RequestLocation),
		UserAgent:       strings.TrimSpace(entry.UserAgent),
		Browser:         strings.TrimSpace(entry.Browser),
		OS:              strings.TrimSpace(entry.OS),
		OperationTime:   &now,
		ExecutionTime:   entry.ExecutionTime,
		Status:          entry.Status,
		ErrorMsg:        strings.TrimSpace(entry.ErrorMsg),
		CreateTime:      &now,
		UpdateTime:      &now,
		IsDeleted:       0,
	}
}

func toOperationLogVOs(items []domain.OperationLog) []adminfacade.OperationLogVO {
	result := make([]adminfacade.OperationLogVO, 0, len(items))
	for _, item := range items {
		result = append(result, toOperationLogVO(item))
	}
	return result
}

func toOperationLogVO(item domain.OperationLog) adminfacade.OperationLogVO {
	opType := adminfacade.OperationTypeEnum(strings.TrimSpace(item.OperationType))
	return adminfacade.OperationLogVO{
		ID:                 item.ID,
		UserID:             item.UserID,
		UserName:           item.UserName,
		NickName:           item.NickName,
		OperationType:      item.OperationType,
		OperationTypeDesc:  opType.Description(),
		OperationTypeLabel: opType.DisplayLabel(item.OperationDesc),
		OperationDesc:      item.OperationDesc,
		MethodName:         item.MethodName,
		RequestMethod:      item.RequestMethod,
		RequestURL:         item.RequestURL,
		TraceID:            item.TraceID,
		RequestParams:      item.RequestParams,
		ResponseResult:     item.ResponseResult,
		RequestIP:          item.RequestIP,
		RequestLocation:    item.RequestLocation,
		UserAgent:          item.UserAgent,
		Browser:            item.Browser,
		OS:                 item.OS,
		OperationTime:      item.OperationTime,
		ExecutionTime:      item.ExecutionTime,
		Status:             item.Status,
		ErrorMsg:           item.ErrorMsg,
		CreateTime:         item.CreateTime,
	}
}

func detectBrowserOS(userAgent string) (string, string) {
	value := strings.ToLower(strings.TrimSpace(userAgent))
	browser := "Unknown"
	switch {
	case strings.Contains(value, "edg/"):
		browser = "Edge"
	case strings.Contains(value, "chrome/"):
		browser = "Chrome"
	case strings.Contains(value, "firefox/"):
		browser = "Firefox"
	case strings.Contains(value, "safari/") && !strings.Contains(value, "chrome/"):
		browser = "Safari"
	}
	os := "Unknown"
	switch {
	case strings.Contains(value, "windows"):
		os = "Windows"
	case strings.Contains(value, "mac os x"):
		os = "macOS"
	case strings.Contains(value, "android"):
		os = "Android"
	case strings.Contains(value, "iphone"), strings.Contains(value, "ipad"), strings.Contains(value, "ios"):
		os = "iOS"
	case strings.Contains(value, "linux"):
		os = "Linux"
	}
	return browser, os
}

func deriveUserRole(info *authorizationfacade.UserVO) string {
	if info == nil {
		return ""
	}
	if info.IsAdmin {
		return "admin"
	}
	if len(info.RoleNames) > 0 && strings.TrimSpace(info.RoleNames[0]) != "" {
		return strings.TrimSpace(info.RoleNames[0])
	}
	if len(info.RoleCodes) > 0 && strings.TrimSpace(info.RoleCodes[0]) != "" {
		return strings.TrimSpace(info.RoleCodes[0])
	}
	return ""
}

func compareSessionActivity(left, right ssofacade.SessionRecord) int {
	leftTime := sessionActivity(left)
	rightTime := sessionActivity(right)
	if leftTime.After(rightTime) {
		return 1
	}
	if leftTime.Before(rightTime) {
		return -1
	}
	return strings.Compare(strings.TrimSpace(left.SessionID), strings.TrimSpace(right.SessionID))
}

func sessionActivity(item ssofacade.SessionRecord) time.Time {
	if item.LastAccessAt != nil {
		return item.LastAccessAt.UTC()
	}
	if item.LoginAt != nil {
		return item.LoginAt.UTC()
	}
	return time.Time{}
}

func mapStatusText(status int) string {
	if status == 1 {
		return "成功"
	}
	return "失败"
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

func ptr[T any](value T) *T {
	return &value
}
