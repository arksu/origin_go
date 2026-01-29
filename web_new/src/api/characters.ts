import client from './client'
import type {
  Character,
  CharacterListResponse,
  CreateCharacterRequest,
  CreateCharacterResponse,
  EnterCharacterResponse,
} from '@/types/api'

export async function listCharacters(): Promise<Character[]> {
  const response = await client.get<CharacterListResponse>('/characters')
  return response.data.list || []
}

export async function createCharacter(data: CreateCharacterRequest): Promise<CreateCharacterResponse> {
  const response = await client.post<CreateCharacterResponse>('/characters', data)
  return response.data
}

export async function deleteCharacter(id: number): Promise<void> {
  await client.delete(`/characters/${id}`)
}

export async function enterCharacter(id: number): Promise<EnterCharacterResponse> {
  const response = await client.post<EnterCharacterResponse>(`/characters/${id}/enter`)
  return response.data
}
