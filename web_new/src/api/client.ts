import axios, { AxiosError, type AxiosInstance } from 'axios'
import { config } from '@/config'
import { ApiErrorType, ApiException } from './errors'

let authStore: { token: string; logout: () => void } | null = null

export function setAuthStore(store: { token: string; logout: () => void }) {
  authStore = store
}

function classifyError(error: AxiosError): ApiException {
  if (!error.response) {
    return new ApiException(
      ApiErrorType.NETWORK,
      'No connection to server',
      0,
    )
  }

  const status = error.response.status
  const data = error.response.data as { error?: string; message?: string; details?: Record<string, string[]> } | undefined

  const message = data?.message || data?.error || 'Unknown error'
  const details = data?.details

  if (status === 400 || status === 422) {
    return new ApiException(ApiErrorType.VALIDATION, message, status, details)
  }

  if (status === 401) {
    return new ApiException(ApiErrorType.AUTH, message, status)
  }

  if (status === 403) {
    return new ApiException(ApiErrorType.FORBIDDEN, message, status)
  }

  if (status >= 500) {
    return new ApiException(ApiErrorType.SERVER, 'Server error', status)
  }

  return new ApiException(ApiErrorType.UNKNOWN, message, status)
}

const client: AxiosInstance = axios.create({
  baseURL: config.API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
  timeout: 10000,
})

client.interceptors.request.use((reqConfig) => {
  if (authStore?.token) {
    reqConfig.headers.Authorization = `Bearer ${authStore.token}`
  }
  return reqConfig
})

client.interceptors.response.use(
  (response) => response,
  (error: AxiosError) => {
    const apiError = classifyError(error)

    if (apiError.type === ApiErrorType.AUTH && authStore) {
      authStore.logout()
    }

    return Promise.reject(apiError)
  },
)

export default client
