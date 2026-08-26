import { afterEach, describe, expect, it, vi } from 'vitest'
import { setAccessToken } from './http'
import { streamLogEvents } from './log-stream'

interface TestEvent {
  message: string
  completed: boolean
}

function streamResponse(chunks: string[]) {
  const encoder = new TextEncoder()
  const reads = chunks.map((chunk) => ({ value: encoder.encode(chunk), done: false as const }))
  const reader = {
    read: vi.fn(async () => reads.shift() ?? { value: undefined, done: true as const }),
  }

  return {
    ok: true,
    status: 200,
    body: { getReader: () => reader },
  } as unknown as Response
}

afterEach(() => {
  setAccessToken('')
  vi.unstubAllGlobals()
})

describe('streamLogEvents', () => {
  it('parses frames split across response chunks and stops on completion', async () => {
    setAccessToken('token')
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        streamResponse([
          'data: {"message":"running","completed":false}\n',
          '\ndata: {"message":"done","completed":true}\n\n',
        ]),
      )
    vi.stubGlobal('fetch', fetchMock)

    const events: TestEvent[] = []
    await new Promise<void>((resolve, reject) => {
      streamLogEvents<TestEvent>('/api/test/logs', {
        onEvent: (event) => events.push(event),
        onDone: resolve,
        onError: (message) => reject(new Error(message)),
      })
    })

    expect(events).toEqual([
      { message: 'running', completed: false },
      { message: 'done', completed: true },
    ])
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/test/logs',
      expect.objectContaining({
        headers: { Authorization: 'Bearer token' },
      }),
    )
  })

  it('forwards an API error message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: async () => ({ message: '没有日志读取权限' }),
      } as Response),
    )

    const message = await new Promise<string>((resolve) => {
      streamLogEvents<TestEvent>('/api/test/logs', {
        onEvent: () => undefined,
        onError: resolve,
      })
    })

    expect(message).toBe('没有日志读取权限')
  })

  it('accepts CRLF-delimited SSE frames', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(streamResponse(['data: {"message":"done","completed":true}\r\n\r\n'])),
    )

    const events: TestEvent[] = []
    await new Promise<void>((resolve, reject) => {
      streamLogEvents<TestEvent>('/api/test/logs', {
        onEvent: (event) => events.push(event),
        onDone: resolve,
        onError: (message) => reject(new Error(message)),
      })
    })

    expect(events).toEqual([{ message: 'done', completed: true }])
  })
})
