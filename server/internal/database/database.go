package database

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"backupx/server/internal/config"
	"backupx/server/internal/model"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func Open(cfg config.DatabaseConfig, logger *zap.Logger) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}

	separator := "?"
	if strings.Contains(cfg.Path, "?") {
		separator = "&"
	}
	// busy_timeout 减少 Agent 轮询、心跳和任务写入同时发生时的瞬时锁错误。
	// 维持默认回滚日志模式，保证当前嵌入式 SQLite 依赖的数据完整性。
	dsn := cfg.Path + separator + "_pragma=busy_timeout(5000)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	initialized := false
	defer func() {
		if initialized {
			return
		}
		sqlDB, dbErr := db.DB()
		if dbErr != nil {
			if logger != nil {
				logger.Warn("get database handle after initialization failure", zap.Error(dbErr))
			}
			return
		}
		if closeErr := sqlDB.Close(); closeErr != nil && logger != nil {
			logger.Warn("close database after initialization failure", zap.Error(closeErr))
		}
	}()

	if err := db.AutoMigrate(&model.User{}, &model.SystemConfig{}, &model.StorageTarget{}, &model.OAuthSession{}, &model.BackupTask{}, &model.BackupRecord{}, &model.Notification{}, &model.Node{}, &model.BackupTaskStorageTarget{}, &model.AuditLog{}, &model.AgentCommand{}, &model.AgentInstallToken{}, &model.RestoreRecord{}, &model.VerificationRecord{}, &model.ApiKey{}, &model.ReplicationRecord{}, &model.TaskTemplate{}); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	// 一次性数据迁移：从 backup_tasks.storage_target_id 回填到多对多中间表
	var count int64
	if err := db.Model(&model.BackupTaskStorageTarget{}).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("count backup task storage target mappings: %w", err)
	}
	if count == 0 {
		if err := db.Exec("INSERT INTO backup_task_storage_targets (backup_task_id, storage_target_id) SELECT id, storage_target_id FROM backup_tasks WHERE storage_target_id > 0").Error; err != nil {
			return nil, fmt.Errorf("backfill backup task storage target mappings: %w", err)
		}
	}

	reconciled, err := reconcileInterruptedOperations(db, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("reconcile interrupted operations: %w", err)
	}
	if reconciled > 0 {
		logger.Warn("interrupted operations marked as failed", zap.Int64("records", reconciled))
	}

	logger.Info("database initialized", zap.String("path", cfg.Path))
	initialized = true
	return db, nil
}

func reconcileInterruptedOperations(db *gorm.DB, completedAt time.Time) (int64, error) {
	const message = "应用在任务完成前重启，执行状态已自动收敛为失败"
	var reconciled int64
	err := db.Transaction(func(tx *gorm.DB) error {
		// Pending/dispatched Agent commands survive a Master restart. Their Agent
		// may still be executing (or may claim the pending command after startup),
		// so their linked records must not be mistaken for orphaned local work.
		var activeCommands []model.AgentCommand
		if err := tx.Where("status IN ? AND type IN ?",
			[]string{model.AgentCommandStatusPending, model.AgentCommandStatusDispatched},
			[]string{model.AgentCommandTypeRunTask, model.AgentCommandTypeRestoreRecord}).
			Find(&activeCommands).Error; err != nil {
			return fmt.Errorf("active agent commands: %w", err)
		}
		activeBackupRecordIDs := make([]uint, 0, len(activeCommands))
		activeRestoreRecordIDs := make([]uint, 0, len(activeCommands))
		for i := range activeCommands {
			cmd := &activeCommands[i]
			switch cmd.Type {
			case model.AgentCommandTypeRunTask:
				var payload struct {
					RecordID uint `json:"recordId"`
				}
				if json.Unmarshal([]byte(cmd.Payload), &payload) == nil && payload.RecordID > 0 {
					activeBackupRecordIDs = append(activeBackupRecordIDs, payload.RecordID)
				}
			case model.AgentCommandTypeRestoreRecord:
				var payload struct {
					RestoreRecordID uint `json:"restoreRecordId"`
				}
				if json.Unmarshal([]byte(cmd.Payload), &payload) == nil && payload.RestoreRecordID > 0 {
					activeRestoreRecordIDs = append(activeRestoreRecordIDs, payload.RestoreRecordID)
				}
			}
		}

		markFailed := func(entity any, runningStatus, failedStatus string, activeAgentRecordIDs []uint) error {
			query := tx.Model(entity).Where("status = ?", runningStatus)
			if len(activeAgentRecordIDs) > 0 {
				query = query.Where("id NOT IN ?", activeAgentRecordIDs)
			}
			result := query.
				Updates(map[string]any{
					"status":           failedStatus,
					"error_message":    message,
					"completed_at":     completedAt,
					"duration_seconds": gorm.Expr("CAST(MAX(0, (julianday(?) - julianday(started_at)) * 86400) AS INTEGER)", completedAt),
				})
			if result.Error != nil {
				return result.Error
			}
			reconciled += result.RowsAffected
			return nil
		}

		if err := markFailed(&model.BackupRecord{}, model.BackupRecordStatusRunning, model.BackupRecordStatusFailed, activeBackupRecordIDs); err != nil {
			return fmt.Errorf("backup records: %w", err)
		}
		if err := markFailed(&model.RestoreRecord{}, model.RestoreRecordStatusRunning, model.RestoreRecordStatusFailed, activeRestoreRecordIDs); err != nil {
			return fmt.Errorf("restore records: %w", err)
		}
		if err := markFailed(&model.VerificationRecord{}, model.VerificationRecordStatusRunning, model.VerificationRecordStatusFailed, nil); err != nil {
			return fmt.Errorf("verification records: %w", err)
		}
		if err := markFailed(&model.ReplicationRecord{}, model.ReplicationStatusRunning, model.ReplicationStatusFailed, nil); err != nil {
			return fmt.Errorf("replication records: %w", err)
		}

		result := tx.Model(&model.BackupTask{}).
			Where("last_status = ? AND NOT EXISTS (SELECT 1 FROM backup_records WHERE backup_records.task_id = backup_tasks.id AND backup_records.status = ?)", model.BackupTaskStatusRunning, model.BackupRecordStatusRunning).
			Update("last_status", model.BackupTaskStatusFailed)
		if result.Error != nil {
			return fmt.Errorf("backup tasks: %w", result.Error)
		}
		reconciled += result.RowsAffected
		return nil
	})
	return reconciled, err
}
