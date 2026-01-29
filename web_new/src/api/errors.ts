export enum ApiErrorType {
  VALIDATION = 'VALIDATION',
  AUTH = 'AUTH',
  FORBIDDEN = 'FORBIDDEN',
  NETWORK = 'NETWORK',
  SERVER = 'SERVER',
  UNKNOWN = 'UNKNOWN',
}

export class ApiException extends Error {
  constructor(
    public readonly type: ApiErrorType,
    message: string,
    public readonly status?: number,
    public readonly details?: Record<string, string[]>,
  ) {
    super(message)
    this.name = 'ApiException'
  }

  static isValidation(error: unknown): error is ApiException {
    return error instanceof ApiException && error.type === ApiErrorType.VALIDATION
  }

  static isAuth(error: unknown): error is ApiException {
    return error instanceof ApiException && error.type === ApiErrorType.AUTH
  }

  static isNetwork(error: unknown): error is ApiException {
    return error instanceof ApiException && error.type === ApiErrorType.NETWORK
  }
}
