import type { HealthResponse } from "@/lib/generated/types"

const BASE = "/api/v1"

export class ProblemError extends Error {
  status: number
  title: string
  detail: string
  requestId: string
  errors?: Record<string, string>

  constructor(status: number, title: string, detail: string, requestId: string, errors?: Record<string, string>) {
    super(detail || title)
    this.name = "ProblemError"
    this.status = status
    this.title = title
    this.detail = detail
    this.requestId = requestId
    this.errors = errors
  }
}

async function request<T>(path: string, envelope: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: "include",
    ...init,
  })

  if (!res.ok) {
    let problem: Partial<ProblemError> = {}
    try {
      problem = await res.json()
    } catch {
      // Non-JSON error body (proxy down, etc.) — fall through to generic problem.
    }
    throw new ProblemError(
      res.status,
      problem.title ?? res.statusText,
      problem.detail ?? "",
      problem.requestId ?? res.headers.get("X-Request-Id") ?? "",
      problem.errors,
    )
  }

  if (res.status === 204) return undefined as T

  const body = await res.json()
  return body[envelope] as T
}

export const api = {
  health: () => request<HealthResponse>("/healthz", "health"),
}
