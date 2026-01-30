import { proto } from './proto/packets.js'
import { config } from '@/config'
import type { ConnectionState, ConnectionError } from './types'

type MessageHandler = (message: proto.ServerMessage) => void
type StateChangeHandler = (state: ConnectionState, error?: ConnectionError) => void

export class GameConnection {
  private ws: WebSocket | null = null
  private state: ConnectionState = 'disconnected'
  private pingInterval: ReturnType<typeof setInterval> | null = null
  private messageHandler: MessageHandler | null = null
  private stateChangeHandler: StateChangeHandler | null = null
  private authToken: string = ''
  private sequence: number = 0

  onMessage(handler: MessageHandler): void {
    this.messageHandler = handler
  }

  onStateChange(handler: StateChangeHandler): void {
    this.stateChangeHandler = handler
  }

  getState(): ConnectionState {
    return this.state
  }

  connect(authToken: string): void {
    if (this.ws) {
      this.disconnect()
    }

    this.authToken = authToken
    this.setState('connecting')

    try {
      this.ws = new WebSocket(config.WS_URL)
      this.ws.binaryType = 'arraybuffer'

      this.ws.onopen = this.handleOpen.bind(this)
      this.ws.onmessage = this.handleMessage.bind(this)
      this.ws.onclose = this.handleClose.bind(this)
      this.ws.onerror = this.handleError.bind(this)
    } catch (err) {
      this.setState('error', {
        code: 'CONNECTION_FAILED',
        message: err instanceof Error ? err.message : 'Failed to connect',
      })
    }
  }

  disconnect(): void {
    this.stopPing()

    if (this.ws) {
      this.ws.onopen = null
      this.ws.onmessage = null
      this.ws.onclose = null
      this.ws.onerror = null
      this.ws.close()
      this.ws = null
    }

    this.setState('disconnected')
  }

  send(payload: proto.IClientMessage): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn('[GameConnection] Cannot send: not connected')
      return
    }

    const message = proto.ClientMessage.create({
      sequence: ++this.sequence,
      ...payload,
    })

    const buffer = proto.ClientMessage.encode(message).finish()
    this.ws.send(buffer)

    if (config.DEBUG && !payload.ping) {
      console.debug('[GameConnection] Sent:', payload)
    }
  }

  sendPing(): void {
    this.send({
      ping: proto.C2S_Ping.create({
        clientTimeMs: Date.now(),
      }),
    })
  }

  private handleOpen(): void {
    this.setState('authenticating')
    this.sendAuth()
  }

  private sendAuth(): void {
    this.send({
      auth: proto.C2S_Auth.create({
        token: this.authToken,
        clientVersion: config.CLIENT_VERSION,
      }),
    })
  }

  private handleMessage(event: MessageEvent): void {
    try {
      const buffer = new Uint8Array(event.data as ArrayBuffer)
      const message = proto.ServerMessage.decode(buffer)

      if (message.authResult) {
        this.handleAuthResult(message.authResult)
        return
      }

      if (message.pong) {
        this.handlePong(message.pong)
        return
      }

      this.messageHandler?.(message)
    } catch (err) {
      console.error('[GameConnection] Failed to decode message:', err)
    }
  }

  private handleAuthResult(result: proto.IS2C_AuthResult): void {
    if (result.success) {
      this.setState('connected')
      this.startPing()
    } else {
      this.setState('error', {
        code: 'AUTH_FAILED',
        message: result.errorMessage || 'Authentication failed',
      })
      this.disconnect()
    }
  }

  private handlePong(pong: proto.IS2C_Pong): void {
    if (config.DEBUG) {
      const latency = Date.now() - Number(pong.clientTimeMs)
      console.debug(`[GameConnection] Pong: latency=${latency}ms`)
    }
  }

  private handleClose(event: CloseEvent): void {
    this.stopPing()

    if (this.state !== 'disconnected' && this.state !== 'error') {
      this.setState('error', {
        code: 'CONNECTION_CLOSED',
        message: event.reason || 'Connection closed',
      })
    }

    this.ws = null
  }

  private handleError(): void {
    if (this.state === 'connecting') {
      this.setState('error', {
        code: 'CONNECTION_FAILED',
        message: 'Failed to establish connection',
      })
    }
  }

  private startPing(): void {
    this.stopPing()
    this.pingInterval = setInterval(() => {
      this.sendPing()
    }, config.PING_INTERVAL_MS)
  }

  private stopPing(): void {
    if (this.pingInterval) {
      clearInterval(this.pingInterval)
      this.pingInterval = null
    }
  }

  private setState(state: ConnectionState, error?: ConnectionError): void {
    this.state = state
    this.stateChangeHandler?.(state, error)
  }
}

export const gameConnection = new GameConnection()
