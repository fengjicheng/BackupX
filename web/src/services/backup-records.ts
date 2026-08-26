import { http, type ApiEnvelope, unwrapApiEnvelope } from './http'
import type {
  BackupLogEvent,
  BackupRecordContents,
  BackupRecordDetail,
  BackupRecordListFilter,
  BackupRecordSummary,
} from '../types/backup-records'
import { streamLogEvents, type LogStreamHandlers } from './log-stream'

function buildRecordQuery(filter: BackupRecordListFilter) {
  const query: Record<string, string | number> = {}
  if (filter.taskId) {
    query.taskId = filter.taskId
  }
  if (filter.status) {
    query.status = filter.status
  }
  if (filter.dateFrom) {
    query.dateFrom = filter.dateFrom
  }
  if (filter.dateTo) {
    query.dateTo = filter.dateTo
  }
  return query
}

function parseContentDisposition(value?: string) {
  if (!value) {
    return 'backup-artifact.bin'
  }
  const match = value.match(/filename="?([^";]+)"?/i)
  return match?.[1] ?? 'backup-artifact.bin'
}

export async function listBackupRecords(filter: BackupRecordListFilter = {}) {
  const response = await http.get<ApiEnvelope<BackupRecordSummary[]>>('/backup/records', {
    params: buildRecordQuery(filter),
  })
  return unwrapApiEnvelope(response.data)
}

export async function getBackupRecord(id: number) {
  const response = await http.get<ApiEnvelope<BackupRecordDetail>>(`/backup/records/${id}`)
  return unwrapApiEnvelope(response.data)
}

// getBackupRecordContents 获取备份记录的文件清单（内容浏览，只读）。
export async function getBackupRecordContents(id: number) {
  const response = await http.get<ApiEnvelope<BackupRecordContents>>(
    `/backup/records/${id}/contents`,
  )
  return unwrapApiEnvelope(response.data)
}

export async function downloadBackupRecord(id: number) {
  const response = await http.get<Blob>(`/backup/records/${id}/download`, { responseType: 'blob' })
  return {
    blob: response.data,
    fileName: parseContentDisposition(response.headers['content-disposition']),
  }
}

export async function deleteBackupRecord(id: number) {
  const response = await http.delete<ApiEnvelope<{ deleted: boolean }>>(`/backup/records/${id}`)
  return unwrapApiEnvelope(response.data)
}

// setBackupRecordLock 设置/解除备份记录的保留锁定（法律保留）。
export async function setBackupRecordLock(id: number, locked: boolean) {
  const response = await http.put<ApiEnvelope<BackupRecordDetail>>(`/backup/records/${id}/lock`, {
    locked,
  })
  return unwrapApiEnvelope(response.data)
}

export function streamBackupRecordLogs(
  recordId: number,
  handlers: LogStreamHandlers<BackupLogEvent>,
) {
  return streamLogEvents(`/api/backup/records/${recordId}/logs/stream`, handlers)
}
