import client from './client'
import type { LoginRequest, LoginResponse, RegisterRequest, RegisterResponse } from '@/types/api'

export async function login(data: LoginRequest): Promise<LoginResponse> {
  const response = await client.post<LoginResponse>('/accounts/login', data)
  return response.data
}

export async function register(data: RegisterRequest): Promise<RegisterResponse> {
  const response = await client.post<RegisterResponse>('/accounts/registration', data)
  return response.data
}
