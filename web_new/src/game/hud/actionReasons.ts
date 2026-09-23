const REASONS: Record<string, string> = {
  ACTION_REQUIRES_SKILL: 'Required skill missing',
  ACTION_REQUIRES_EQUIPMENT: 'Required equipment missing',
  LOW_STAMINA: 'Not enough stamina',
  LIFT_ALREADY_CARRYING: 'Already carrying an object',
  LIFT_NOT_CARRYING: 'Carry an object first',
  ACTION_UNAVAILABLE: 'Action unavailable',
}

export function actionUnavailableReason(reasonCode: string | null | undefined): string {
  if (!reasonCode) return 'Unavailable'
  return REASONS[reasonCode] || reasonCode.toLowerCase().replaceAll('_', ' ')
}
