export interface LoginRequest {
  login: string
  password: string
}

export interface LoginResponse {
  token: string
}

export interface RegisterRequest {
  login: string
  password: string
}

export interface RegisterResponse {
  id: number
  login: string
}

export interface Character {
  id: number
  name: string
  created_at?: string
}

export interface CharacterListResponse {
  list: Character[]
}

export interface CreateCharacterRequest {
  name: string
}

export interface CreateCharacterResponse {
  id: number
  name: string
}

export interface EnterCharacterResponse {
  ws_token: string
  character_id: number
}

export interface ApiError {
  error: string
  message?: string
  details?: Record<string, string[]>
}
