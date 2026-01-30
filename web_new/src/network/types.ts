export type ConnectionState =
  | 'disconnected'
  | 'connecting'
  | 'authenticating'
  | 'connected'
  | 'error'

export interface ConnectionError {
  code: string
  message: string
}
