import client from './client'

export async function listCharacters() {
  const response = await client.get('/characters')
  return response.data
}

export async function createCharacter(name) {
  const response = await client.post('/characters', { name })
  return response.data
}

export async function deleteCharacter(id) {
  const response = await client.delete(`/characters/${id}`)
  return response.data
}

export async function enterCharacter(id) {
  const response = await client.post(`/characters/${id}/enter`)
  return response.data
}
