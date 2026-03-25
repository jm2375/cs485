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
    const cached = localStorage.getItem('bootstrap_v1');
    if (cached) {
      const data: BootstrapResult = JSON.parse(cached);
      setToken(data.token);
      setTripId(data.tripId);
      return data;
    }
    const data = await req<BootstrapResult>('POST', '/api/dev/bootstrap');
    localStorage.setItem('bootstrap_v1', JSON.stringify(data));
    setToken(data.token);
    setTripId(data.tripId);
    return data;
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
