import type {
  CreateCustomerRequest,
  CustomerListResponse,
  CustomerResponse,
  HealthResponse,
  LoginRequest,
  SignupRequest,
  SignupResponse,
  UpdateCustomerRequest,
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

// send performs the fetch and turns any non-2xx into a ProblemError (with the
// 401 → login redirect side effect). Both request() and requestBody() build on
// it so the error handling lives in exactly one place.
async function send(path: string, init?: RequestInit): Promise<Response> {
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

  return res
}

// request unwraps a single named envelope key, e.g. body.customer. Use for
// single-entity responses and void (204) endpoints.
async function request<T>(path: string, envelope: string, init?: RequestInit): Promise<T> {
  const res = await send(path, init)
  if (res.status === 204) return undefined as T
  const body = await res.json()
  return body[envelope] as T
}

// requestBody returns the whole JSON body. Use for list responses that carry
// several top-level keys (e.g. { customers, metadata }).
async function requestBody<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await send(path, init)
  return (await res.json()) as T
}

// customerListQuery builds the list URL query string from typed params.
export type CustomerListQuery = {
  page: number
  pageSize: number
  search: string
  archived: boolean
}

function customerListPath(q: CustomerListQuery): string {
  const params = new URLSearchParams()
  params.set("page", String(q.page))
  params.set("pageSize", String(q.pageSize))
  if (q.search) params.set("search", q.search)
  if (q.archived) params.set("archived", "true")
  return `/customers?${params.toString()}`
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

  customers: {
    list: (q: CustomerListQuery) =>
      requestBody<CustomerListResponse>(customerListPath(q)),
    get: (id: string) => request<CustomerResponse>(`/customers/${id}`, "customer"),
    create: (data: CreateCustomerRequest) =>
      request<CustomerResponse>("/customers", "customer", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }),
    update: (id: string, data: UpdateCustomerRequest) =>
      request<CustomerResponse>(`/customers/${id}`, "customer", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }),
    archive: (id: string) =>
      request<void>(`/customers/${id}/archive`, "", { method: "POST" }),
  },
}
