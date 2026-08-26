package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"backupx/server/internal/apperror"
	"backupx/server/internal/model"
	"backupx/server/internal/repository"
	"go.uber.org/zap"
)

// AuditEntry 是记录审计日志的输入结构
type AuditEntry struct {
	UserID     uint
	Username   string
	Category   string // auth / storage_target / backup_task / backup_record / settings
	Action     string // create / update / delete / login_success / login_failed / ...
	TargetType string
	TargetID   string
	TargetName string
	Detail     string
	ClientIP   string
}

type AuditService struct {
	repo repository.AuditLogRepository

	// webhook 外输配置（可选）
	webhookMu     sync.RWMutex
	webhookURL    string
	webhookSecret string
	httpClient    *http.Client
	async         func(func(context.Context)) bool
	logger        *zap.Logger
	inFlight      chan struct{}
}

const maxAuditInFlight = 64

func NewAuditService(repo repository.AuditLogRepository) *AuditService {
	return &AuditService{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 3 * time.Second, // 短超时：审计 webhook 不应拖慢业务
		},
		async:    runDetached,
		logger:   zap.NewNop(),
		inFlight: make(chan struct{}, maxAuditInFlight),
	}
}

func (s *AuditService) SetLogger(logger *zap.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

// SetBackgroundRunner binds audit persistence and webhook delivery to the application lifecycle.
func (s *AuditService) SetBackgroundRunner(runner BackgroundRunner) {
	if runner != nil {
		s.async = runner.Go
	}
}

// PurgeOlderThan 删除早于 days 天前的审计日志，返回删除条数。days<=0 时不清理。
func (s *AuditService) PurgeOlderThan(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	return s.repo.DeleteBefore(ctx, cutoff)
}

// StartRetentionMonitor 启动后台审计保留期清理：按 interval 周期读取
// audit_retention_days 设置，>0 时删除超期审计日志。缺省/0 表示永久保留
// （向后兼容，默认不删任何历史）。ctx 取消后退出。
func (s *AuditService) StartRetentionMonitor(ctx context.Context, configs repository.SystemConfigRepository, interval time.Duration) {
	if s == nil || configs == nil {
		return
	}
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	if ctx == nil {
		ctx = context.Background()
	}
	accepted := s.async(func(workerCtx context.Context) {
		monitorCtx, cancel := context.WithCancel(workerCtx)
		defer cancel()
		stopLink := context.AfterFunc(ctx, cancel)
		defer stopLink()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		s.runRetentionOnce(monitorCtx, configs) // 启动后立即跑一次
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-ticker.C:
				s.runRetentionOnce(monitorCtx, configs)
			}
		}
	})
	if !accepted {
		s.logger.Warn("audit retention monitor not started: application is shutting down")
	}
}

func (s *AuditService) runRetentionOnce(ctx context.Context, configs repository.SystemConfigRepository) {
	cfg, err := configs.GetByKey(ctx, SettingKeyAuditRetentionDays)
	if err != nil {
		s.logger.Warn("read audit retention setting failed", zap.Error(err))
		return
	}
	if cfg == nil {
		return
	}
	days, err := strconv.Atoi(strings.TrimSpace(cfg.Value))
	if err != nil {
		s.logger.Warn("invalid audit retention setting", zap.String("value", cfg.Value), zap.Error(err))
		return
	}
	if days <= 0 {
		return
	}
	deleted, err := s.PurgeOlderThan(ctx, days)
	if err != nil {
		s.logger.Warn("audit retention purge failed", zap.Error(err))
		return
	}
	if deleted > 0 {
		s.logger.Info("audit retention purge completed", zap.Int64("deleted", deleted), zap.Int("retention_days", days))
	}
}

// SetWebhook 动态配置审计事件转发 URL 与签名密钥。
//   - url 为空字符串时禁用转发
//   - secret 非空时对 payload 计算 HMAC-SHA256，作为 X-BackupX-Signature header
//
// 适用场景：
//   - 企业 SIEM 集成（Splunk HEC、ELK、Loki）
//   - 安全审计留痕到第三方 WORM 存储
//   - 合规日志归档（GDPR / SOC2）
func (s *AuditService) SetWebhook(url, secret string) {
	if s == nil {
		return
	}
	s.webhookMu.Lock()
	defer s.webhookMu.Unlock()
	s.webhookURL = strings.TrimSpace(url)
	s.webhookSecret = strings.TrimSpace(secret)
}

// Record asynchronously persists an audit event without blocking the request.
func (s *AuditService) Record(entry AuditEntry) {
	if s == nil || s.repo == nil {
		return
	}
	select {
	case s.inFlight <- struct{}{}:
	default:
		s.logger.Error("audit event rejected: in-flight limit reached",
			zap.Int("limit", cap(s.inFlight)),
			zap.String("category", entry.Category),
			zap.String("action", entry.Action))
		return
	}
	accepted := s.async(func(workerCtx context.Context) {
		defer func() { <-s.inFlight }()
		persistCtx, cancel := finalizationContext(workerCtx)
		defer cancel()
		record := &model.AuditLog{
			UserID:     entry.UserID,
			Username:   entry.Username,
			Category:   entry.Category,
			Action:     entry.Action,
			TargetType: entry.TargetType,
			TargetID:   entry.TargetID,
			TargetName: entry.TargetName,
			Detail:     entry.Detail,
			ClientIP:   entry.ClientIP,
		}
		if err := s.repo.Create(persistCtx, record); err != nil {
			s.logger.Error("failed to write audit log", zap.String("category", entry.Category), zap.String("action", entry.Action), zap.Error(err))
		}
		if err := s.fireWebhook(persistCtx, record); err != nil {
			s.logger.Warn("audit webhook delivery failed", zap.String("category", entry.Category), zap.String("action", entry.Action), zap.Error(err))
		}
	})
	if !accepted {
		<-s.inFlight
		s.logger.Warn("audit event rejected: application is shutting down", zap.String("category", entry.Category), zap.String("action", entry.Action))
	}
}

// fireWebhook forwards an audit event. The caller owns asynchronous execution.
func (s *AuditService) fireWebhook(ctx context.Context, record *model.AuditLog) error {
	if s == nil {
		return nil
	}
	s.webhookMu.RLock()
	url := s.webhookURL
	secret := s.webhookSecret
	s.webhookMu.RUnlock()
	if url == "" {
		return nil
	}
	payload := map[string]any{
		"eventType":  "audit.log",
		"occurredAt": record.CreatedAt.UTC().Format(time.RFC3339),
		"actor": map[string]any{
			"userId":   record.UserID,
			"username": record.Username,
		},
		"category":   record.Category,
		"action":     record.Action,
		"targetType": record.TargetType,
		"targetId":   record.TargetID,
		"targetName": record.TargetName,
		"detail":     record.Detail,
		"clientIp":   record.ClientIP,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal audit webhook: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build audit webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "BackupX-Audit/1.0")
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-BackupX-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post audit webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("audit webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// List 分页查询审计日志
func (s *AuditService) List(ctx context.Context, category string, limit, offset int) (*repository.AuditLogListResult, error) {
	result, err := s.repo.List(ctx, repository.AuditLogListOptions{
		Category: category,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, apperror.Internal("AUDIT_LOG_LIST_FAILED", fmt.Sprintf("无法获取审计日志列表: %v", err), err)
	}
	return result, nil
}

// ListAdvanced 多字段筛选分页查询（合规审计常用）。
func (s *AuditService) ListAdvanced(ctx context.Context, opts repository.AuditLogListOptions) (*repository.AuditLogListResult, error) {
	result, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, apperror.Internal("AUDIT_LOG_LIST_FAILED", fmt.Sprintf("无法获取审计日志: %v", err), err)
	}
	return result, nil
}

// ExportAll 返回指定筛选条件下的全部审计日志（最多 10000 条），用于 CSV 导出。
func (s *AuditService) ExportAll(ctx context.Context, opts repository.AuditLogListOptions) ([]model.AuditLog, error) {
	items, err := s.repo.ListAll(ctx, opts)
	if err != nil {
		return nil, apperror.Internal("AUDIT_LOG_EXPORT_FAILED", fmt.Sprintf("无法导出审计日志: %v", err), err)
	}
	return items, nil
}
