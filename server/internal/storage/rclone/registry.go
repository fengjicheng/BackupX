package rclone

import "backupx/server/internal/storage"

// NewDefaultRegistry returns the storage factory set shared by Master, Agent,
// and the standalone Backint process.
func NewDefaultRegistry() *storage.Registry {
	registry := storage.NewRegistry(
		NewLocalDiskFactory(),
		NewS3Factory(),
		NewWebDAVFactory(),
		NewGoogleDriveFactory(),
		NewAliyunOSSFactory(),
		NewTencentCOSFactory(),
		NewQiniuKodoFactory(),
		NewFTPFactory(),
		NewRcloneFactory(),
	)
	RegisterAllBackends(registry)
	return registry
}
