package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"backupx/server/internal/backup"
	"go.uber.org/zap"
)

// Agent 是 Agent 进程的主控制器。
type Agent struct {
	cfg      *Config
	client   *MasterClient
	executor *Executor
	version  string
	logger   *zap.Logger

	mu      sync.Mutex
	started bool
}

// New 构造 Agent。
func New(cfg *Config, version string) (*Agent, error) {
	if err := cfg.ResolveToken(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	client := NewMasterClient(cfg.Master, cfg.Token, cfg.InsecureSkipTLSVerify)
	if err := client.ConfigureTransport(cfg.ProxyURL, cfg.CACertFile); err != nil {
		return nil, fmt.Errorf("configure master connection: %w", err)
	}
	executor := NewExecutor(client, cfg.TempDir)
	return &Agent{
		cfg:      cfg,
		client:   client,
		executor: executor,
		version:  version,
		logger:   zap.NewNop(),
	}, nil
}

// SetLogger attaches the process logger used by the Agent runtime loop.
func (a *Agent) SetLogger(logger *zap.Logger) {
	if logger != nil {
		a.logger = logger
		if a.executor != nil {
			a.executor.SetLogger(logger)
		}
	}
}

// Run 启动 Agent 主循环，阻塞直到 ctx 被取消。
func (a *Agent) Run(ctx context.Context) error {
	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		return fmt.Errorf("agent already started")
	}
	a.started = true
	a.mu.Unlock()

	hbInterval := parseDuration(a.cfg.HeartbeatInterval, 15*time.Second)
	pollInterval := parseDuration(a.cfg.PollInterval, 5*time.Second)

	// 首次握手：通过一次心跳确认 token 有效
	if err := a.heartbeatOnce(ctx); err != nil {
		return fmt.Errorf("initial heartbeat failed: %w", err)
	}
	a.logger.Info("agent connected to master", zap.String("master", a.cfg.Master))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		a.heartbeatLoop(ctx, hbInterval)
	}()
	go func() {
		defer wg.Done()
		a.pollLoop(ctx, pollInterval)
	}()
	wg.Wait()
	return ctx.Err()
}

// heartbeatLoop 定期发送心跳。
func (a *Agent) heartbeatLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.heartbeatOnce(ctx); err != nil {
				a.logger.Warn("agent heartbeat failed", zap.Error(err))
			}
		}
	}
}

func (a *Agent) heartbeatOnce(ctx context.Context) error {
	hostname, err := os.Hostname()
	if err != nil {
		a.logger.Warn("resolve agent hostname failed", zap.Error(err))
	}
	req := HeartbeatRequest{
		Hostname:     hostname,
		IPAddress:    detectLocalIP(),
		AgentVersion: a.version,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
	}
	_, err = a.client.Heartbeat(ctx, req)
	return err
}

// pollLoop 定期拉取并处理待执行命令。
func (a *Agent) pollLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.pollAndHandleOnce(ctx)
		}
	}
}

func (a *Agent) pollAndHandleOnce(ctx context.Context) {
	cmd, err := a.client.PollCommand(ctx)
	if err != nil {
		a.logger.Warn("poll agent command failed", zap.Error(err))
		return
	}
	if cmd == nil {
		return
	}
	a.logger.Info("agent command received", zap.Uint("command_id", cmd.ID), zap.String("command_type", cmd.Type))
	switch cmd.Type {
	case "run_task":
		a.handleRunTask(ctx, cmd)
	case "list_dir":
		a.handleListDir(ctx, cmd)
	case "restore_record":
		a.handleRestoreRecord(ctx, cmd)
	case "discover_db":
		a.handleDiscoverDB(ctx, cmd)
	case "delete_storage_object":
		a.handleDeleteStorageObject(ctx, cmd)
	default:
		msg := fmt.Sprintf("unknown command type: %s", cmd.Type)
		a.logger.Warn("unknown agent command", zap.Uint("command_id", cmd.ID), zap.String("command_type", cmd.Type))
		a.submitCommandResult(ctx, cmd.ID, false, msg, nil)
	}
}

// handleRunTask 处理 run_task 命令
func (a *Agent) handleRunTask(ctx context.Context, cmd *CommandPayload) {
	var payload struct {
		TaskID   uint `json:"taskId"`
		RecordID uint `json:"recordId"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, "invalid payload: "+err.Error(), nil)
		return
	}
	if err := a.executor.ExecuteRunTask(ctx, payload.TaskID, payload.RecordID); err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, err.Error(), nil)
		return
	}
	a.submitCommandResult(ctx, cmd.ID, true, "", map[string]any{
		"taskId":   payload.TaskID,
		"recordId": payload.RecordID,
	})
}

// handleRestoreRecord 处理 restore_record 命令
func (a *Agent) handleRestoreRecord(ctx context.Context, cmd *CommandPayload) {
	var payload struct {
		RestoreRecordID uint `json:"restoreRecordId"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, "invalid payload: "+err.Error(), nil)
		return
	}
	if payload.RestoreRecordID == 0 {
		a.submitCommandResult(ctx, cmd.ID, false, "restoreRecordId is required", nil)
		return
	}
	if err := a.executor.ExecuteRestore(ctx, payload.RestoreRecordID); err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, err.Error(), nil)
		return
	}
	a.submitCommandResult(ctx, cmd.ID, true, "", map[string]any{
		"restoreRecordId": payload.RestoreRecordID,
	})
}

// handleDeleteStorageObject 处理 delete_storage_object 命令：在 Agent 侧删除指定存储对象。
// 用于跨节点 local_disk 场景下的远程备份文件清理。
func (a *Agent) handleDeleteStorageObject(ctx context.Context, cmd *CommandPayload) {
	var payload struct {
		TargetType   string         `json:"targetType"`
		TargetConfig map[string]any `json:"targetConfig"`
		StoragePath  string         `json:"storagePath"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, "invalid payload: "+err.Error(), nil)
		return
	}
	if strings.TrimSpace(payload.StoragePath) == "" {
		a.submitCommandResult(ctx, cmd.ID, false, "storagePath is required", nil)
		return
	}
	provider, err := a.executor.storageRegistry.Create(ctx, payload.TargetType, payload.TargetConfig)
	if err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, "create provider: "+err.Error(), nil)
		return
	}
	if err := provider.Delete(ctx, payload.StoragePath); err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, "delete object: "+err.Error(), nil)
		return
	}
	a.submitCommandResult(ctx, cmd.ID, true, "", map[string]any{"deleted": true})
}

// handleDiscoverDB 处理 discover_db 命令：在 Agent 本机执行 mysql/psql 列出数据库。
func (a *Agent) handleDiscoverDB(ctx context.Context, cmd *CommandPayload) {
	var payload struct {
		Type     string `json:"type"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, "invalid payload: "+err.Error(), nil)
		return
	}
	databases, err := backup.DiscoverDatabases(ctx, backup.NewOSCommandExecutor(), backup.DiscoverRequest{
		Type:     payload.Type,
		Host:     payload.Host,
		Port:     payload.Port,
		User:     payload.User,
		Password: payload.Password,
	})
	if err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, err.Error(), nil)
		return
	}
	a.submitCommandResult(ctx, cmd.ID, true, "", map[string]any{"databases": databases})
}

// handleListDir 处理 list_dir 命令（阶段四实现）
func (a *Agent) handleListDir(ctx context.Context, cmd *CommandPayload) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, "invalid payload: "+err.Error(), nil)
		return
	}
	entries, err := listLocalDir(payload.Path)
	if err != nil {
		a.submitCommandResult(ctx, cmd.ID, false, err.Error(), nil)
		return
	}
	a.submitCommandResult(ctx, cmd.ID, true, "", map[string]any{"entries": entries})
}

func (a *Agent) submitCommandResult(ctx context.Context, commandID uint, success bool, message string, data any) {
	reportCtx, cancel := agentFinalizationContext(ctx)
	defer cancel()
	var err error
	attempts := 0
retryLoop:
	for attempts < 3 {
		attempts++
		err = a.client.SubmitCommandResult(reportCtx, commandID, success, message, data)
		if err == nil {
			return
		}
		if attempts < 3 {
			timer := time.NewTimer(time.Duration(attempts) * 100 * time.Millisecond)
			select {
			case <-reportCtx.Done():
				timer.Stop()
				break retryLoop
			case <-timer.C:
			}
		}
	}
	a.logger.Error("submit agent command result failed",
		zap.Uint("command_id", commandID),
		zap.Bool("success", success),
		zap.Int("attempts", attempts),
		zap.Error(err))
}

// 辅助函数

func parseDuration(s string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return fallback
}

func detectLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return ""
}
