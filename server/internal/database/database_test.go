package database

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"backupx/server/internal/config"
	"backupx/server/internal/logger"
	"backupx/server/internal/model"
)

func TestOpenConfiguresSQLiteForSingleMasterConcurrency(t *testing.T) {
	log, err := logger.New(config.LogConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "backupx.db")}, log)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var journalMode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		t.Fatal(err)
	}
	if journalMode != "delete" {
		t.Fatalf("journal_mode = %q, want delete", journalMode)
	}
	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}
}

func TestReconcileInterruptedOperations(t *testing.T) {
	log, err := logger.New(config.LogConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "reconcile.db")}, log)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	startedAt := time.Now().UTC().Add(-time.Minute)
	task := model.BackupTask{Name: "interrupted", Type: model.BackupTaskTypeFile, LastStatus: model.BackupTaskStatusRunning}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	items := []any{
		&model.BackupRecord{TaskID: task.ID, Status: model.BackupRecordStatusRunning, StartedAt: startedAt},
		&model.RestoreRecord{TaskID: task.ID, Status: model.RestoreRecordStatusRunning, StartedAt: startedAt},
		&model.VerificationRecord{TaskID: task.ID, Status: model.VerificationRecordStatusRunning, StartedAt: startedAt},
		&model.ReplicationRecord{TaskID: task.ID, Status: model.ReplicationStatusRunning, StartedAt: startedAt},
	}
	for _, item := range items {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}

	completedAt := time.Now().UTC()
	count, err := reconcileInterruptedOperations(db, completedAt)
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("reconciled records = %d, want 5", count)
	}

	var runningRecords int64
	for _, entity := range []any{&model.BackupRecord{}, &model.RestoreRecord{}, &model.VerificationRecord{}, &model.ReplicationRecord{}} {
		if err := db.Model(entity).Where("status = ?", "running").Count(&runningRecords).Error; err != nil {
			t.Fatal(err)
		}
		if runningRecords != 0 {
			t.Fatalf("%T still has %d running records", entity, runningRecords)
		}
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.LastStatus != model.BackupTaskStatusFailed {
		t.Fatalf("task last status = %q, want failed", task.LastStatus)
	}
	var backupRecord model.BackupRecord
	if err := db.Where("task_id = ?", task.ID).First(&backupRecord).Error; err != nil {
		t.Fatal(err)
	}
	if backupRecord.DurationSeconds <= 0 {
		t.Fatalf("backup duration = %d, want positive duration", backupRecord.DurationSeconds)
	}
}

func TestReconcileInterruptedOperationsPreservesRemoteAgentWork(t *testing.T) {
	log, err := logger.New(config.LogConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "remote-reconcile.db")}, log)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	localNode := model.Node{Name: "local", Token: "local-token", Status: model.NodeStatusOnline, IsLocal: true}
	remoteNode := model.Node{Name: "remote", Token: "remote-token", Status: model.NodeStatusOnline, IsLocal: false}
	if err := db.Create(&localNode).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&remoteNode).Error; err != nil {
		t.Fatal(err)
	}
	localTask := model.BackupTask{Name: "local-interrupted", Type: model.BackupTaskTypeFile, LastStatus: model.BackupTaskStatusRunning}
	remoteTask := model.BackupTask{Name: "remote-still-running", Type: model.BackupTaskTypeFile, LastStatus: model.BackupTaskStatusRunning}
	orphanRemoteTask := model.BackupTask{Name: "remote-without-command", Type: model.BackupTaskTypeFile, LastStatus: model.BackupTaskStatusRunning}
	if err := db.Create(&localTask).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&remoteTask).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&orphanRemoteTask).Error; err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().UTC().Add(-time.Minute)
	localBackup := &model.BackupRecord{TaskID: localTask.ID, NodeID: localNode.ID, Status: model.BackupRecordStatusRunning, StartedAt: startedAt}
	localRestore := &model.RestoreRecord{TaskID: localTask.ID, NodeID: localNode.ID, Status: model.RestoreRecordStatusRunning, StartedAt: startedAt}
	remoteBackup := &model.BackupRecord{TaskID: remoteTask.ID, NodeID: remoteNode.ID, Status: model.BackupRecordStatusRunning, StartedAt: startedAt}
	remoteRestore := &model.RestoreRecord{TaskID: remoteTask.ID, NodeID: remoteNode.ID, Status: model.RestoreRecordStatusRunning, StartedAt: startedAt}
	orphanRemoteBackup := &model.BackupRecord{TaskID: orphanRemoteTask.ID, NodeID: remoteNode.ID, Status: model.BackupRecordStatusRunning, StartedAt: startedAt}
	items := []any{localBackup, localRestore, remoteBackup, remoteRestore, orphanRemoteBackup}
	for _, item := range items {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	activeCommands := []model.AgentCommand{
		{NodeID: remoteNode.ID, Type: model.AgentCommandTypeRunTask, Status: model.AgentCommandStatusDispatched, Payload: fmt.Sprintf(`{"recordId":%d}`, remoteBackup.ID)},
		{NodeID: remoteNode.ID, Type: model.AgentCommandTypeRestoreRecord, Status: model.AgentCommandStatusPending, Payload: fmt.Sprintf(`{"restoreRecordId":%d}`, remoteRestore.ID)},
	}
	if err := db.Create(&activeCommands).Error; err != nil {
		t.Fatal(err)
	}

	count, err := reconcileInterruptedOperations(db, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("reconciled rows = %d, want local and unlinked remote records plus their tasks", count)
	}

	for _, entity := range []any{&model.BackupRecord{}, &model.RestoreRecord{}} {
		var remoteRunning int64
		if err := db.Model(entity).Where("task_id = ? AND status = ?", remoteTask.ID, "running").Count(&remoteRunning).Error; err != nil {
			t.Fatal(err)
		}
		if remoteRunning != 1 {
			t.Fatalf("%T remote running records = %d, want 1", entity, remoteRunning)
		}
		var localRunning int64
		if err := db.Model(entity).Where("task_id = ? AND status = ?", localTask.ID, "running").Count(&localRunning).Error; err != nil {
			t.Fatal(err)
		}
		if localRunning != 0 {
			t.Fatalf("%T local running records = %d, want 0", entity, localRunning)
		}
	}
	if err := db.First(&localTask, localTask.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&remoteTask, remoteTask.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&orphanRemoteTask, orphanRemoteTask.ID).Error; err != nil {
		t.Fatal(err)
	}
	if localTask.LastStatus != model.BackupTaskStatusFailed || remoteTask.LastStatus != model.BackupTaskStatusRunning {
		t.Fatalf("task statuses = local:%q remote:%q", localTask.LastStatus, remoteTask.LastStatus)
	}
	if orphanRemoteTask.LastStatus != model.BackupTaskStatusFailed {
		t.Fatalf("unlinked remote task status = %q, want failed", orphanRemoteTask.LastStatus)
	}
	if err := db.First(orphanRemoteBackup, orphanRemoteBackup.ID).Error; err != nil {
		t.Fatal(err)
	}
	if orphanRemoteBackup.Status != model.BackupRecordStatusFailed {
		t.Fatalf("unlinked remote backup status = %q, want failed", orphanRemoteBackup.Status)
	}
}
