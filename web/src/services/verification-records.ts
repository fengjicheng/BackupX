import { http, type ApiEnvelope, unwrapApiEnvelope } from './http'
import type { BackupLogEvent } from '../types/backup-records'
import type {
  VerificationMode,
  VerificationRecordDetail,
  VerificationRecordListFilter,
  VerificationRecordSummary,
} from '../types/verification-records'
import { streamLogEvents, type LogStreamHandlers } from './log-stream'

function buildQuery(filter: VerificationRecordListFilter) {
  const query: Record<string, string | number> = {}
  if (filter.taskId) query.taskId = filter.taskId
  if (filter.backupRecordId) query.backupRecordId = filter.backupRecordId
  if (filter.status) query.status = filter.status
  if (filter.dateFrom) query.dateFrom = filter.dateFrom
  if (filter.dateTo) query.dateTo = filter.dateTo
  return query
}

export async function listVerificationRecords(filter: VerificationRecordListFilter = {}) {
  const response = await http.get<ApiEnvelope<VerificationRecordSummary[]>>('/verify/records', {
    params: buildQuery(filter),
  })
  return unwrapApiEnvelope(response.data)
}

export async function getVerificationRecord(id: number) {
  const response = await http.get<ApiEnvelope<VerificationRecordDetail>>(`/verify/records/${id}`)
  return unwrapApiEnvelope(response.data)
}

// startVerifyByTask 使用任务的最新成功备份触发验证。
export async function startVerifyByTask(taskId: number, mode: VerificationMode = 'quick') {
  const response = await http.post<ApiEnvelope<VerificationRecordDetail>>(
    `/backup/tasks/${taskId}/verify`,
    { mode },
  )
  return unwrapApiEnvelope(response.data)
}

// startVerifyByRecord 指定备份记录触发验证。
export async function startVerifyByRecord(
  backupRecordId: number,
  mode: VerificationMode = 'quick',
) {
  const response = await http.post<ApiEnvelope<VerificationRecordDetail>>(
    `/backup/records/${backupRecordId}/verify`,
    { mode },
  )
  return unwrapApiEnvelope(response.data)
}

export function streamVerificationRecordLogs(
  verifyId: number,
  handlers: LogStreamHandlers<BackupLogEvent>,
) {
  return streamLogEvents(`/api/verify/records/${verifyId}/logs/stream`, handlers)
}
