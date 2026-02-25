import type { ObjectEditorInitResponse, PreviewDiffResponse } from '@/types/objectEditor'

export async function loadObjectEditorInit(): Promise<ObjectEditorInitResponse> {
  const res = await fetch('/__api/object-editor/init')
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`Init failed: ${res.status} ${text}`)
  }
  return await res.json() as ObjectEditorInitResponse
}

export async function previewObjectDiff(fileName: string, json: unknown): Promise<PreviewDiffResponse> {
  const res = await fetch('/__api/object-editor/preview-diff', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ fileName, json }),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`Diff failed: ${res.status} ${text}`)
  }
  return await res.json() as PreviewDiffResponse
}
