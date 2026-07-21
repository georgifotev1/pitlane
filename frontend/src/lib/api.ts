import type {
  HealthResponse,
  LoginRequest,
  SignupRequest,
  SignupResponse,
  UserResponse,
} from "@/lib/generated/types"

const BASE = "/api/v1"

export class ProblemError extends Error {
  status: number
  title: string
  detail: string
  requestId: string
  code?: string
  errors?: Record<string, string>

  constructor(
    status: number,
    title: string,
    detail: string,
    requestId: string,
    code?: string,
    errors?: Record<string, string>,
  ) {
    super(detail || title)
    this.name = "ProblemError"
    this.status = status
    this.title = title
    this.detail = detail
    this.requestId = requestId
    this.code = code
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
    const pe = new ProblemError(
      res.status,
      problem.title ?? res.statusText,
      problem.detail ?? "",
      problem.requestId ?? res.headers.get("X-Request-Id") ?? "",
      problem.code,
      problem.errors,
    )

    if (res.status === 401 && typeof window !== "undefined") {
      // Redirect unauthenticated requests to login, preserving the destination
      // for post-login navigation.
      const here = window.location.pathname + window.location.search
      const target = `/login?redirect=${encodeURIComponent(here)}`
      // Avoid infinite redirects on the login page itself.
      if (!window.location.pathname.startsWith("/login")) {
        window.location.href = target
      }
    }
    throw pe
  }

  if (res.status === 204) return undefined as T

  const body = await res.json()
  return body[envelope] as T
}

export const api = {
  health: () => request<HealthResponse>("/healthz", "health"),
  signup: (data: SignupRequest) =>
    request<SignupResponse>("/auth/signup", "user", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    }),
  login: (data: LoginRequest) =>
    request<void>("/auth/login", "", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    }),
  logout: () => request<void>("/auth/logout", "", { method: "POST" }),
  me: () => request<UserResponse>("/auth/me", "user"),
}
