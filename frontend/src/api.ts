/**
 * api.ts — typed client for the Go backend.
 *
 * Auth flow (dev / P4):
 *   1. On first use, POST /api/dev/bootstrap to get a JWT + demo tripId.
 *   2. Both are persisted in localStorage so page reloads stay authenticated.
 *   3. Every subsequent request carries  Authorization: Bearer <token>.
 */

import type { Trip, POI, ItineraryItem, Role } from './types';

// ── Colour palette for collaborators that arrive without one ─────────────────
const COLLAB_COLORS = [
  '#7C3AED', '#2563EB', '#059669', '#D97706',
  '#DC2626', '#6366F1', '#EC4899', '#14B8A6',
];

// ── Internal storage helpers ─────────────────────────────────────────────────

function getToken(): string | null { return localStorage.getItem('auth_token'); }
function setToken(t: string): void  { localStorage.setItem('auth_token', t); }
function getTripId(): string | null { return localStorage.getItem('trip_id'); }
function setTripId(id: string): void { localStorage.setItem('trip_id', id); }

// ── Core fetch wrapper ───────────────────────────────────────────────────────

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const token = getToken();
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
  // 204 No Content
  if (res.status === 204) return undefined as unknown as T;
  return res.json() as Promise<T>;
}

// ── Shape returned by /api/dev/bootstrap ────────────────────────────────────

interface BootstrapResult {
  tripId: string;
  token:  string;
  userId: string;
}

// Raw collaborator shape from backend (no color / avatarUrl yet)
interface RawCollaborator {
  id:          string;
  name:        string;
  email:       string;
  role:        Role;
  isOnline:    boolean;
  joinedAt:    string;
}

interface RawTrip {
  id:            string;
  name:          string;
  destination:   string;
  shareLink:     string;
  collaborators: RawCollaborator[];
}

// ── Public API surface ───────────────────────────────────────────────────────

export const api = {

  /**
   * Returns the cached tripId from localStorage, or null if bootstrap has not
   * been called yet.  Call bootstrap() first on app mount.
   */
  getCachedTripId(): string | null { return getTripId(); },

  /**
   * Authenticates as the demo owner and returns the demo trip ID + JWT.
   * Results are cached in localStorage so repeat calls are instant.
   */
  async bootstrap(): Promise<BootstrapResult> {
    // 1. Returning Sarah Chen session — use cached bootstrap data.
    const cached = localStorage.getItem('bootstrap_v1');
    if (cached) {
      const data: BootstrapResult = JSON.parse(cached);
      setToken(data.token);
      setTripId(data.tripId);
      return data;
    }

    // 2. A real user is logged in — validate their token first.
    const existingToken = getToken();
    if (existingToken) {
      try {
        const user = await req<{ id: string }>('GET', '/api/auth/me');
        const existingTripId = getTripId();
        if (existingTripId) {
          // Real user with a known trip — use their credentials directly.
          return { token: existingToken, tripId: existingTripId, userId: user.id };
        }
        // Real user but no trip yet (e.g. just logged in) — borrow the demo
        // trip ID without touching the user's token or caching bootstrap_v1.
        const demo = await req<BootstrapResult>('POST', '/api/dev/bootstrap');
        setTripId(demo.tripId);
        return { token: existingToken, tripId: demo.tripId, userId: user.id };
      } catch {
        // Token is expired or invalid — clear it and fall through.
        localStorage.removeItem('auth_token');
      }
    }

    // 3. No authenticated user — bootstrap as the demo owner (Sarah Chen).
    const data = await req<BootstrapResult>('POST', '/api/dev/bootstrap');
    localStorage.setItem('bootstrap_v1', JSON.stringify(data));
    setToken(data.token);
    setTripId(data.tripId);
    return data;
  },

  /** Fetch the currently authenticated user's profile. */
  async getCurrentUser(): Promise<{ id: string; email: string; displayName: string }> {
    return req('GET', '/api/auth/me');
  },

  /** Full trip detail including collaborators with online status. */
  async getTrip(tripId: string): Promise<Trip> {
    const raw = await req<RawTrip>('GET', `/api/trips/${tripId}`);
    return {
      ...raw,
      collaborators: raw.collaborators.map((c, i) => ({
        ...c,
        color: COLLAB_COLORS[i % COLLAB_COLORS.length],
      })),
    };
  },

  /** Itinerary items for a trip. */
  async getItinerary(tripId: string): Promise<ItineraryItem[]> {
    const res = await req<{ items: ItineraryItem[] }>('GET', `/api/trips/${tripId}/itinerary`);
    return res.items ?? [];
  },

  /** Add a POI to a trip's itinerary. Returns the new ItineraryItem. */
  async addToItinerary(tripId: string, poiId: string, day: number, notes = ''): Promise<ItineraryItem> {
    return req<ItineraryItem>('POST', `/api/trips/${tripId}/itinerary`, { poiId, day, notes });
  },

  /** Remove an item from the itinerary. */
  async removeFromItinerary(tripId: string, itemId: string): Promise<void> {
    await req<void>('DELETE', `/api/trips/${tripId}/itinerary/${itemId}`);
  },

  /** Search POIs by text and/or category near a destination. */
  async searchPOIs(query: string, category: string, near?: string): Promise<POI[]> {
    const params = new URLSearchParams();
    if (query)    params.set('q', query);
    if (category && category !== 'all') params.set('category', category);
    if (near)     params.set('near', near);
    const res = await req<{ pois: POI[] }>('GET', `/api/pois/search?${params.toString()}`);
    return res.pois ?? [];
  },

  /** Send email invitations for a trip. */
  async sendInvites(tripId: string, emails: string[], role: Extract<Role, 'Editor' | 'Viewer'>): Promise<void> {
    await req<unknown>('POST', `/api/trips/${tripId}/invitations`, { emails, role });
  },

  /** Update a collaborator's role. */
  async updateCollaboratorRole(tripId: string, userId: string, role: Role): Promise<void> {
    await req<unknown>('PATCH', `/api/trips/${tripId}/collaborators/${userId}`, { role });
  },

  /** Remove a collaborator from a trip. */
  async removeCollaborator(tripId: string, userId: string): Promise<void> {
    await req<void>('DELETE', `/api/trips/${tripId}/collaborators/${userId}`);
  },

  /** Returns true if a JWT token is stored locally. */
  isAuthenticated(): boolean { return !!getToken(); },

  /** Clear all locally stored auth state (logout). */
  logout(): void {
    localStorage.removeItem('auth_token');
    localStorage.removeItem('trip_id');
    localStorage.removeItem('bootstrap_v1');
  },

  /** Log in with email + password. Stores the token and clears any bootstrap cache. */
  async login(email: string, password: string): Promise<{ token: string; user: { id: string; email: string; displayName: string } }> {
    const data = await req<{ token: string; user: { id: string; email: string; displayName: string } }>(
      'POST', '/api/auth/login', { email, password }
    );
    localStorage.removeItem('bootstrap_v1');
    localStorage.removeItem('trip_id');
    setToken(data.token);
    return data;
  },

  /** Register a new account. Stores the token and clears any bootstrap cache. */
  async register(email: string, displayName: string, password: string): Promise<{ token: string; user: { id: string; email: string; displayName: string } }> {
    const data = await req<{ token: string; user: { id: string; email: string; displayName: string } }>(
      'POST', '/api/auth/register', { email, displayName, password }
    );
    localStorage.removeItem('bootstrap_v1');
    localStorage.removeItem('trip_id');
    setToken(data.token);
    return data;
  },

  /** Preview an email invitation token (public — no auth needed). */
  async getInvitePreview(token: string): Promise<{ invitationId: string; tripId: string; tripName: string; destination: string; role: string; expiresAt: string }> {
    return req('GET', `/api/invitations/accept/${encodeURIComponent(token)}`);
  },

  /** Accept an email invitation (requires auth). */
  async acceptInvitation(token: string): Promise<void> {
    await req('POST', `/api/invitations/accept/${encodeURIComponent(token)}`);
  },

  /** Preview a shareable invite link (public — no auth needed). */
  async getShareLinkPreview(inviteCode: string): Promise<{ tripId: string; name: string; destination: string }> {
    return req('GET', `/api/join/${encodeURIComponent(inviteCode)}`);
  },

  /** Join a trip via shareable link (requires auth). */
  async joinByInviteCode(inviteCode: string): Promise<void> {
    await req('POST', `/api/join/${encodeURIComponent(inviteCode)}`);
  },

  /**
   * Opens a WebSocket connection to /ws for the given trip.
   * The JWT is passed as a query parameter (standard practice when
   * the Authorization header cannot be set on WebSocket upgrade requests).
   */
  createWSConnection(tripId: string): WebSocket {
    const token = getToken() ?? '';
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    return new WebSocket(`${proto}://${location.host}/ws?token=${encodeURIComponent(token)}&tripId=${encodeURIComponent(tripId)}`);
  },
};
