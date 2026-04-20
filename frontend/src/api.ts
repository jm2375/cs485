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

function getToken(): string | null  { return localStorage.getItem('auth_token'); }
function setToken(t: string): void   { localStorage.setItem('auth_token', t); }
function getTripId(): string | null  { return localStorage.getItem('trip_id'); }
function setTripId(id: string): void { localStorage.setItem('trip_id', id); }
function getStoredUserId(): string | null { return localStorage.getItem('user_id'); }
function setStoredUserId(id: string): void { localStorage.setItem('user_id', id); }

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
   * Validates the stored token and returns the current user + tripId.
   * If no tripId is cached locally, fetches the user's trips from the backend
   * and restores the most recent one. Throws if unauthenticated or token expired.
   */
  async bootstrap(): Promise<{ token: string; tripId: string | null; userId: string }> {
    const existingToken = getToken();
    if (!existingToken) {
      throw new Error('Not authenticated');
    }

    // Validate token — only this failure should clear the stored token.
    let user: { id: string };
    try {
      user = await req<{ id: string }>('GET', '/api/auth/me');
    } catch {
      localStorage.removeItem('auth_token');
      throw new Error('Session expired');
    }

    return { token: existingToken, tripId: getTripId(), userId: user.id };
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
    localStorage.removeItem('user_id');
    localStorage.removeItem('bootstrap_v1');
  },

  /** Log in with email + password. Stores the token and clears any bootstrap cache. */
  async login(email: string, password: string): Promise<{ token: string; user: { id: string; email: string; displayName: string } }> {
    const data = await req<{ token: string; user: { id: string; email: string; displayName: string } }>(
      'POST', '/api/auth/login', { email, password }
    );
    localStorage.removeItem('bootstrap_v1');
    // If a different user is logging in, clear the previous user's trip.
    if (getStoredUserId() && getStoredUserId() !== data.user.id) {
      localStorage.removeItem('trip_id');
    }
    setToken(data.token);
    setStoredUserId(data.user.id);
    return data;
  },

  /** Register a new account. Stores the token and clears any bootstrap cache. */
  async register(email: string, displayName: string, password: string): Promise<{ token: string; user: { id: string; email: string; displayName: string } }> {
    const data = await req<{ token: string; user: { id: string; email: string; displayName: string } }>(
      'POST', '/api/auth/register', { email, displayName, password }
    );
    localStorage.removeItem('bootstrap_v1');
    // New account — no trip yet; clear any leftover trip from a previous user.
    localStorage.removeItem('trip_id');
    setToken(data.token);
    setStoredUserId(data.user.id);
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

  /** Update a trip's name and/or destination. */
  async updateTrip(tripId: string, fields: { name?: string; destination?: string }): Promise<void> {
    await req<unknown>('PATCH', `/api/trips/${tripId}`, fields);
  },

  /** Fetch all trips the current user is a member of. */
  async listTrips(): Promise<{ id: string; name: string; destination: string }[]> {
    const { trips } = await req<{ trips: { id: string; name: string; destination: string }[] }>('GET', '/api/trips');
    return trips ?? [];
  },

  /** Persist a chosen trip ID to localStorage (used by the trip selector). */
  selectTrip(id: string): void {
    setTripId(id);
  },

  /** Create a new trip. Stores the new trip ID locally and returns it. */
  async createTrip(name: string, destination: string): Promise<{ id: string }> {
    const data = await req<{ id: string }>('POST', '/api/trips', { name, destination });
    setTripId(data.id);
    return data;
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
