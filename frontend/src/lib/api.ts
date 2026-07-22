import type {
  AttachmentListResponse,
  AttachmentResponse,
  CarListResponse,
  CarResponse,
  CreateCarRequest,
  CreateCustomerRequest,
  CreateHistoryNoteRequest,
  CreateOfferRequest,
  CustomerListResponse,
  CustomerResponse,
  HealthResponse,
  HistoryNoteResponse,
  HistoryResponse,
  LoginRequest,
  OfferListResponse,
  OfferResponse,
  RepairListResponse,
  RepairResponse,
  SignupRequest,
  SignupResponse,
  UpdateCarRequest,
  UpdateCustomerRequest,
  UpdateHistoryNoteRequest,
  UpdateOfferRequest,
  UpdateRepairRequest,
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

// Cars reuse the same list-query shape as customers; list/create are nested
// under a customer, so those take the customerId as their first argument.
export type CarListQuery = {
  page: number
  pageSize: number
  search: string
  archived: boolean
}

function carListPath(customerId: string, q: CarListQuery): string {
  const params = new URLSearchParams()
  params.set("page", String(q.page))
  params.set("pageSize", String(q.pageSize))
  if (q.search) params.set("search", q.search)
  if (q.archived) params.set("archived", "true")
  return `/customers/${customerId}/cars?${params.toString()}`
}

// Offers are nested under a car and have no search/archive filters — just
// pagination — so their list query is simpler than customers'/cars'.
export type OfferListQuery = {
  page: number
  pageSize: number
}

function offerListPath(carId: string, q: OfferListQuery): string {
  const params = new URLSearchParams()
  params.set("page", String(q.page))
  params.set("pageSize", String(q.pageSize))
  return `/cars/${carId}/offers?${params.toString()}`
}

// Repairs are a tenant-wide board (not nested under a car), filtered by an
// optional status. "" means all statuses.
export type RepairListQuery = {
  page: number
  pageSize: number
  status: string
}

function repairListPath(q: RepairListQuery): string {
  const params = new URLSearchParams()
  params.set("page", String(q.page))
  params.set("pageSize", String(q.pageSize))
  if (q.status) params.set("status", q.status)
  return `/repairs?${params.toString()}`
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

  cars: {
    list: (customerId: string, q: CarListQuery) =>
      requestBody<CarListResponse>(carListPath(customerId, q)),
    get: (id: string) => request<CarResponse>(`/cars/${id}`, "car"),
    create: (customerId: string, data: CreateCarRequest) =>
      request<CarResponse>(`/customers/${customerId}/cars`, "car", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }),
    update: (id: string, data: UpdateCarRequest) =>
      request<CarResponse>(`/cars/${id}`, "car", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }),
    archive: (id: string) =>
      request<void>(`/cars/${id}/archive`, "", { method: "POST" }),
  },

  offers: {
    list: (carId: string, q: OfferListQuery) =>
      requestBody<OfferListResponse>(offerListPath(carId, q)),
    get: (id: string) => request<OfferResponse>(`/offers/${id}`, "offer"),
    create: (carId: string, data: CreateOfferRequest) =>
      request<OfferResponse>(`/cars/${carId}/offers`, "offer", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }),
    update: (id: string, data: UpdateOfferRequest) =>
      request<OfferResponse>(`/offers/${id}`, "offer", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }),
    // pdfUrl is the API path for an offer's PDF. The browser hits it directly
    // (iframe src / download anchor) so the session cookie rides along — no
    // fetch+blob dance (ADR §13). `inline` requests the in-browser preview
    // disposition; omit it for a download.
    pdfUrl: (id: string, opts?: { inline?: boolean }) =>
      `${BASE}/offers/${id}/pdf${opts?.inline ? "?disposition=inline" : ""}`,
    // status is the post-send lifecycle endpoint (accepted|rejected|expired).
    // It cannot send — that is a separate, email-backed action below.
    setStatus: (id: string, status: string) =>
      request<OfferResponse>(`/offers/${id}/status`, "offer", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status }),
      }),
    // send emails the offer PDF to the recipient (freeze-on-send). It is also
    // the retry path for a failed delivery. Returns the offer with its updated
    // sendStatus so the UI reflects the pending state immediately.
    send: (id: string, recipient: string) =>
      request<OfferResponse>(`/offers/${id}/send`, "offer", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ recipient }),
      }),
    // accept converts a sent offer into a repair (the sole accept path) and
    // returns the newly created repair, so the caller can navigate to it.
    accept: (id: string) =>
      request<RepairResponse>(`/offers/${id}/accept`, "repair", { method: "POST" }),
  },

  repairs: {
    list: (q: RepairListQuery) => requestBody<RepairListResponse>(repairListPath(q)),
    get: (id: string) => request<RepairResponse>(`/repairs/${id}`, "repair"),
    update: (id: string, data: UpdateRepairRequest) =>
      request<RepairResponse>(`/repairs/${id}`, "repair", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }),
    // status drives the generic lifecycle (open ↔ in_progress).
    setStatus: (id: string, status: string) =>
      request<RepairResponse>(`/repairs/${id}/status`, "repair", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status }),
      }),
    // complete finishes the repair, recording the odometer reading (also written
    // onto the car). It is the sole path to `completed`.
    complete: (id: string, mileage: number) =>
      request<RepairResponse>(`/repairs/${id}/complete`, "repair", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ mileage }),
      }),
    attachments: {
      list: (repairId: string) =>
        requestBody<AttachmentListResponse>(`/repairs/${repairId}/attachments`),
      upload: (repairId: string, file: File) =>
        uploadAttachment(`/repairs/${repairId}/attachments`, file),
    },
  },

  history: {
    list: (carId: string) => requestBody<HistoryResponse>(`/cars/${carId}/history`),
    createNote: (carId: string, data: CreateHistoryNoteRequest) =>
      request<HistoryNoteResponse>(`/cars/${carId}/history/notes`, "note", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }),
    updateNote: (id: string, data: UpdateHistoryNoteRequest) =>
      request<HistoryNoteResponse>(`/history/notes/${id}`, "note", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }),
    deleteNote: (id: string) =>
      request<void>(`/history/notes/${id}`, "", { method: "DELETE" }),
  },

  attachments: {
    listByCar: (carId: string) =>
      requestBody<AttachmentListResponse>(`/cars/${carId}/attachments`),
    uploadToCar: (carId: string, file: File) =>
      uploadAttachment(`/cars/${carId}/attachments`, file),
    downloadUrl: (id: string) => `${BASE}/attachments/${id}`,
    delete: (id: string) =>
      request<void>(`/attachments/${id}`, "", { method: "DELETE" }),
  },
}

// uploadAttachment streams a File as multipart/form-data to the given path.
function uploadAttachment(path: string, file: File): Promise<AttachmentResponse> {
  const body = new FormData()
  body.set("file", file)
  return request<AttachmentResponse>(path, "attachment", {
    method: "POST",
    body,
  })
}
