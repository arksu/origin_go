import type {
  SaveObjectEditorRequest,
  SaveObjectEditorResponse,
} from '@/types/objectEditor'

export async function saveObjectEditorPayload(payload: SaveObjectEditorRequest): Promise<SaveObjectEditorResponse> {
  const res = await fetch('/__api/object-editor/save', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    let msg = `Save failed: ${res.status}`
    try {
      const data = await res.json()
      msg = data.error ?? msg
    } catch {
      // ignore invalid JSON error responses
    }
    throw new Error(msg)
  }

  return await res.json() as SaveObjectEditorResponse
}
