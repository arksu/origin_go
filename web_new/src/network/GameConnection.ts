import { proto } from './proto/packets.js'
import { config } from '@/config'
import type { ConnectionState, ConnectionError } from './types'
import { timeSync } from './TimeSync'
import { serverConstants } from './ServerConstants'
import { gameCalendarSync } from './GameCalendarSync'

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

    this.resetConnectionTime()
    this.authToken = authToken
    this.setState('connecting')

    try {
      const socket = new WebSocket(config.WS_URL)
      this.ws = socket
      socket.binaryType = 'arraybuffer'

      socket.onopen = () => { if (this.ws === socket) this.handleOpen() }
      socket.onmessage = event => { if (this.ws === socket) this.handleMessage(event) }
      socket.onclose = event => { if (this.ws === socket) this.handleClose(event) }
      socket.onerror = () => { if (this.ws === socket) this.handleError() }
    } catch (err) {
      this.setState('error', {
        code: 'CONNECTION_FAILED',
        message: err instanceof Error ? err.message : 'Failed to connect',
      })
    }
  }

  disconnect(): void {
    this.stopPing()
    this.closeSocket()

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

      if (this.state !== 'connected') return

      if (message.serverConstants) {
        const result = serverConstants.accept(message.serverConstants)
        if (result === 'invalid' || result === 'changed') {
          this.protocolError('Invalid or changed server constants')
          return
        }
        if (result === 'accepted') gameCalendarSync.configure(serverConstants.requireSnapshot())
        return
      }

      if (message.pong) {
        this.handlePong(message.pong)
        return
      }

      if (!serverConstants.isReady() && !message.error && !message.warning) {
        this.protocolError('World bootstrap arrived before server constants')
        return
      }
      this.messageHandler?.(message)
    } catch (err) {
      console.error('[GameConnection] Failed to decode message:', err)
    }
  }

  private handleAuthResult(result: proto.IS2C_AuthResult): void {
    if (this.state !== 'authenticating') return
    if (result.success) {
      this.setState('connected')
      this.startPing()
    } else {
      this.setState('error', {
        code: 'AUTH_FAILED',
        message: result.errorMessage || 'Authentication failed',
      })
      // Close socket without overriding the error state in UI.
      this.stopPing()
      this.closeSocket()
    }
  }

  private handlePong(pong: proto.IS2C_Pong): void {
    const clientSendMs = Number(pong.clientTimeMs)
    const serverTimeMs = Number(pong.serverTimeMs)

    if (Number.isSafeInteger(clientSendMs) && Number.isSafeInteger(serverTimeMs) && clientSendMs >= 0 && serverTimeMs >= 0) {
      timeSync.onPong(clientSendMs, serverTimeMs)
    }
    gameCalendarSync.acceptSample(pong.runtimeSecondsTotal, pong.serverTimeMs)

    if (config.DEBUG) {
      const metrics = timeSync.getDebugMetrics()
      console.debug(`[GameConnection] Pong: rtt=${metrics.rttMs}ms, jitter=${metrics.jitterMs}ms, offset=${metrics.offsetMs}ms`)
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
    this.stopPing()
    this.setState('error', {
      code: 'CONNECTION_FAILED',
      message: 'Connection failed',
    })
    this.closeSocket()
  }

  protocolError(message: string): void {
    this.stopPing()
    this.setState('error', { code: 'CONNECTION_FAILED', message })
    this.closeSocket()
  }

  private resetConnectionTime(): void {
    timeSync.reset()
    serverConstants.reset()
    gameCalendarSync.reset()
  }

  private startPing(): void {
    this.stopPing()
    this.sendPing()
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
    if (state === 'disconnected' || state === 'error') this.resetConnectionTime()
    this.state = state
    this.stateChangeHandler?.(state, error)
  }

  private closeSocket(): void {
    if (!this.ws) {
      return
    }
    this.ws.onopen = null
    this.ws.onmessage = null
    this.ws.onclose = null
    this.ws.onerror = null
    this.ws.close()
    this.ws = null
  }
}

export const gameConnection = new GameConnection()
