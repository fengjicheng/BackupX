import { resolveErrorMessage } from '../utils/error'
import { getAccessToken } from './http'

interface CompletionEvent {
  completed: boolean
}

export interface LogStreamHandlers<T> {
  onEvent: (event: T) => void
  onDone?: () => void
  onError?: (message: string) => void
}

function parseLogEvent<T>(frame: string) {
  const payloadLine = frame.split(/\r?\n/).find((line) => line.startsWith('data:'))
  if (!payloadLine) {
    return null
  }

  const payload = payloadLine.slice(5).trim()
  if (!payload) {
    return null
  }

  return JSON.parse(payload) as T
}

function findFrameBoundary(buffer: string) {
  const match = /\r?\n\r?\n/.exec(buffer)
  if (!match) {
    return null
  }
  return { index: match.index, length: match[0].length }
}

async function resolveStreamError(response: Response) {
  try {
    const payload = (await response.json()) as { message?: string }
    return payload.message ?? '连接日志流失败'
  } catch {
    return `连接日志流失败（HTTP ${response.status}）`
  }
}

export function streamLogEvents<T extends CompletionEvent>(
  url: string,
  handlers: LogStreamHandlers<T>,
) {
  const controller = new AbortController()

  void (async () => {
    try {
      const token = getAccessToken()
      const response = await fetch(url, {
        method: 'GET',
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
        signal: controller.signal,
      })

      if (!response.ok) {
        throw new Error(await resolveStreamError(response))
      }
      if (!response.body) {
        throw new Error('日志流不可用')
      }

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { value, done } = await reader.read()
        if (done) {
          break
        }

        buffer += decoder.decode(value, { stream: true })
        while (true) {
          const boundary = findFrameBoundary(buffer)
          if (!boundary) {
            break
          }
          const frame = buffer.slice(0, boundary.index)
          buffer = buffer.slice(boundary.index + boundary.length)
          const event = parseLogEvent<T>(frame)
          if (!event) {
            continue
          }

          handlers.onEvent(event)
          if (event.completed) {
            handlers.onDone?.()
            controller.abort()
            return
          }
        }
      }

      if (buffer.trim()) {
        const event = parseLogEvent<T>(buffer)
        if (event) {
          handlers.onEvent(event)
        }
      }
      handlers.onDone?.()
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        return
      }
      handlers.onError?.(resolveErrorMessage(error, '日志流连接失败'))
    }
  })()

  return () => controller.abort()
}
