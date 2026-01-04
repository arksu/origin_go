import client from './client'

export async function login(login, password) {
  const response = await client.post('/accounts/login', { login, password })
  return response.data
}

export async function register(login, password) {
  const response = await client.post('/accounts/registration', { login, password })
  return response.data
}
