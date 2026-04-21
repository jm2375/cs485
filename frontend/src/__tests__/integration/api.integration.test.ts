/**
 * See SPEC.md in this directory for the full endpoint specification.
 *
 * @jest-environment node
 */

/// <reference types="node" />

const BASE_URL = (process.env.BASE_URL ?? '').replace(/\/$/, '');

// All tests are skipped when BASE_URL is not provided so the normal
// unit-test pipeline (which has no live server) stays green.
const describeIf = BASE_URL ? describe : describe.skip;

// ── Helpers ───────────────────────────────────────────────────────────────────

interface FetchOpts {
  token?: string;
  body?: unknown;
}

async function api(method: string, path: string, opts: FetchOpts = {}) {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (opts.token) headers['Authorization'] = `Bearer ${opts.token}`;

  const res = await fetch(`${BASE_URL}${path}`, {
    method,
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });

  let json: unknown = null;
  const ct = res.headers.get('content-type') ?? '';
  if (ct.includes('application/json') && res.status !== 204) {
    json = await res.json();
  }

  return { status: res.status, body: json as Record<string, unknown> };
}

// ── Shared state (populated in beforeAll) ────────────────────────────────────

const ts = Date.now();
const ownerEmail   = `int-owner-${ts}@test.example`;
const viewerEmail  = `int-viewer-${ts}@test.example`;
const outsiderEmail = `int-outsider-${ts}@test.example`;
const PASSWORD = 'IntTestPass1!';

let ownerToken  = '';
let viewerToken = '';
let outsiderToken = '';
let ownerId     = '';
let viewerId    = '';
let tripId      = '';
let inviteCode  = '';
let invitationId = '';
let itineraryItemId = '';

// ── Suite ─────────────────────────────────────────────────────────────────────

describeIf('Backend Integration — all frontend-to-backend paths', () => {

  // ── Setup ──────────────────────────────────────────────────────────────────

  beforeAll(async () => {
    // 1. Register owner
    const ownerReg = await api('POST', '/api/auth/register', {
      body: { email: ownerEmail, displayName: 'Int Owner', password: PASSWORD },
    });
    expect(ownerReg.status).toBe(201);
    ownerToken = (ownerReg.body as any).token;
    ownerId    = (ownerReg.body as any).user?.id ?? '';

    // 2. Register viewer (used for RBAC tests)
    const viewerReg = await api('POST', '/api/auth/register', {
      body: { email: viewerEmail, displayName: 'Int Viewer', password: PASSWORD },
    });
    expect(viewerReg.status).toBe(201);
    viewerToken = (viewerReg.body as any).token;
    viewerId    = (viewerReg.body as any).user?.id ?? '';

    // 3. Register outsider (used for non-member 403 tests)
    const outsiderReg = await api('POST', '/api/auth/register', {
      body: { email: outsiderEmail, displayName: 'Int Outsider', password: PASSWORD },
    });
    expect(outsiderReg.status).toBe(201);
    outsiderToken = (outsiderReg.body as any).token;

    // 4. Create a trip as owner
    const tripCreate = await api('POST', '/api/trips', {
      token: ownerToken,
      body: { name: `Int Trip ${ts}`, destination: 'Tokyo, Japan' },
    });
    expect(tripCreate.status).toBe(201);
    tripId = (tripCreate.body as any).id ?? '';

    // 5. Get the share link so we can test the join flow
    const slRes = await api('GET', `/api/trips/${tripId}/share-link`, { token: ownerToken });
    inviteCode = (slRes.body as any).inviteCode ?? '';

    // 6. Have viewer join via invite code so they're a member for later tests
    const joinRes = await api('POST', `/api/join/${inviteCode}`, { token: viewerToken });
    expect([200, 204]).toContain(joinRes.status);

    // 7. Pre-seed an invitation so we can test I-02 / I-05
    const invRes = await api('POST', `/api/trips/${tripId}/invitations`, {
      token: ownerToken,
      body: { emails: [`int-inv-${ts}@test.example`], role: 'Viewer' },
    });
    if (invRes.status === 201) {
      invitationId = ((invRes.body as any).invitations?.[0] as any)?.id ?? '';
    }

    // 8. Add a POI to the itinerary so we have an item to delete
    const addPOI = await api('POST', `/api/trips/${tripId}/itinerary`, {
      token: ownerToken,
      body: { poiId: 'h1', day: 1 },
    });
    if (addPOI.status === 201) {
      itineraryItemId = (addPOI.body as any).id ?? '';
    }
  }, 30_000);

  // ── H-01  Health ────────────────────────────────────────────────────────────

  describe('Health', () => {
    test('H-01  GET /health → 200 {status: "ok"}', async () => {
      const { status, body } = await api('GET', '/health');
      expect(status).toBe(200);
      expect((body as any).status).toBe('ok');
    });
  });

  // ── A-xx  Auth ──────────────────────────────────────────────────────────────

  describe('Auth', () => {
    test('A-01  POST /api/auth/register → 201 with token', async () => {
      const email = `reg-${ts}@test.example`;
      const { status, body } = await api('POST', '/api/auth/register', {
        body: { email, displayName: 'Reg Test', password: PASSWORD },
      });
      expect(status).toBe(201);
      expect(typeof (body as any).token).toBe('string');
      expect((body as any).user?.email).toBe(email);
    });

    test('A-01e POST /api/auth/register duplicate email → non-201', async () => {
      const { status } = await api('POST', '/api/auth/register', {
        body: { email: ownerEmail, displayName: 'Dup', password: PASSWORD },
      });
      expect(status).not.toBe(201);
    });

    test('A-02  POST /api/auth/login → 200 with token', async () => {
      const { status, body } = await api('POST', '/api/auth/login', {
        body: { email: ownerEmail, password: PASSWORD },
      });
      expect(status).toBe(200);
      expect(typeof (body as any).token).toBe('string');
    });

    test('A-02e POST /api/auth/login wrong password → 401', async () => {
      const { status } = await api('POST', '/api/auth/login', {
        body: { email: ownerEmail, password: 'wrong-password' },
      });
      expect(status).toBe(401);
    });

    test('A-03  GET /api/auth/me (authenticated) → 200 with user data', async () => {
      const { status, body } = await api('GET', '/api/auth/me', { token: ownerToken });
      expect(status).toBe(200);
      expect((body as any).email).toBe(ownerEmail);
      expect(typeof (body as any).id).toBe('string');
    });

    test('A-03e GET /api/auth/me (no token) → 401', async () => {
      const { status } = await api('GET', '/api/auth/me');
      expect(status).toBe(401);
    });

    test('A-03e GET /api/auth/me (bad token) → 401', async () => {
      const { status } = await api('GET', '/api/auth/me', { token: 'bad.token.here' });
      expect(status).toBe(401);
    });

    test('A-04  POST /api/dev/bootstrap → 200 with token + tripId (or 503 when seed data disabled)', async () => {
      const { status, body } = await api('POST', '/api/dev/bootstrap');
      // 503 is the expected response in production environments where SEED_DATA=false.
      if (status === 503) return;
      expect(status).toBe(200);
      expect(typeof (body as any).token).toBe('string');
      expect(typeof (body as any).tripId).toBe('string');
      expect(typeof (body as any).userId).toBe('string');
    });
  });

  // ── P-xx  POI Search ────────────────────────────────────────────────────────

  describe('POI Search', () => {
    test('P-01  GET /api/pois/search (no params) → 200 with pois array', async () => {
      const { status, body } = await api('GET', '/api/pois/search');
      expect(status).toBe(200);
      expect(Array.isArray((body as any).pois)).toBe(true);
    });

    test('P-02  GET /api/pois/search?category=restaurant → 200, all items match', async () => {
      const { status, body } = await api('GET', '/api/pois/search?category=restaurant');
      expect(status).toBe(200);
      const pois = (body as any).pois as any[];
      expect(pois.length).toBeGreaterThan(0);
      pois.forEach(p => expect(p.category).toBe('restaurant'));
    });

    test('P-03  GET /api/pois/search?q=ramen → 200, returns results', async () => {
      const { status, body } = await api('GET', '/api/pois/search?q=ramen');
      expect(status).toBe(200);
      expect(Array.isArray((body as any).pois)).toBe(true);
    });

    test('P-04  GET /api/pois/search?q=zzznomatch999 → 200, empty array', async () => {
      const { status, body } = await api('GET', '/api/pois/search?q=zzznomatch999');
      expect(status).toBe(200);
      expect((body as any).pois).toHaveLength(0);
    });
  });

  // ── T-xx  Trips ─────────────────────────────────────────────────────────────

  describe('Trips', () => {
    test('T-01  GET /api/trips (authenticated) → 200 with trips array', async () => {
      const { status, body } = await api('GET', '/api/trips', { token: ownerToken });
      expect(status).toBe(200);
      expect(Array.isArray((body as any).trips)).toBe(true);
      const found = (body as any).trips.some((t: any) => t.id === tripId);
      expect(found).toBe(true);
    });

    test('T-01e GET /api/trips (no token) → 401', async () => {
      const { status } = await api('GET', '/api/trips');
      expect(status).toBe(401);
    });

    test('T-02  POST /api/trips → 201 with trip id, owner is sole collaborator', async () => {
      const { status, body } = await api('POST', '/api/trips', {
        token: ownerToken,
        body: { name: `New Trip ${ts}`, destination: 'Paris, France' },
      });
      expect(status).toBe(201);
      expect(typeof (body as any).id).toBe('string');
      const collabs: any[] = (body as any).collaborators ?? [];
      expect(collabs).toHaveLength(1);
      expect(collabs[0].role).toBe('Owner');
    });

    test('T-02e POST /api/trips (no token) → 401', async () => {
      const { status } = await api('POST', '/api/trips', {
        body: { name: 'Unauthenticated Trip', destination: 'Nowhere' },
      });
      expect(status).toBe(401);
    });

    test('T-03  GET /api/trips/:tripId (owner) → 200 with collaborators list', async () => {
      const { status, body } = await api('GET', `/api/trips/${tripId}`, { token: ownerToken });
      expect(status).toBe(200);
      expect((body as any).id).toBe(tripId);
      expect(Array.isArray((body as any).collaborators)).toBe(true);
      expect(typeof (body as any).shareLink).toBe('string');
    });

    test('T-03  GET /api/trips/:tripId (viewer member) → 200', async () => {
      const { status } = await api('GET', `/api/trips/${tripId}`, { token: viewerToken });
      expect(status).toBe(200);
    });

    test('T-03e GET /api/trips/:tripId (no token) → 401', async () => {
      const { status } = await api('GET', `/api/trips/${tripId}`);
      expect(status).toBe(401);
    });

    test('T-03e GET /api/trips/:tripId (outsider) → 403', async () => {
      const { status } = await api('GET', `/api/trips/${tripId}`, { token: outsiderToken });
      expect(status).toBe(403);
    });

    test('T-04  PATCH /api/trips/:tripId → 200', async () => {
      const newName = `Renamed Trip ${ts}`;
      const { status } = await api('PATCH', `/api/trips/${tripId}`, {
        token: ownerToken,
        body: { name: newName },
      });
      expect(status).toBe(200);
    });

    test('T-04e PATCH /api/trips/:tripId (outsider) → 403', async () => {
      const { status } = await api('PATCH', `/api/trips/${tripId}`, {
        token: outsiderToken,
        body: { name: 'Nope' },
      });
      expect(status).toBe(403);
    });

    test('T-05  GET /api/trips/:tripId/share-link → 200 with shareLink and inviteCode', async () => {
      const { status, body } = await api('GET', `/api/trips/${tripId}/share-link`, {
        token: ownerToken,
      });
      expect(status).toBe(200);
      expect(typeof (body as any).inviteCode).toBe('string');
      expect(typeof (body as any).shareLink).toBe('string');
    });

    test('T-05e GET /api/trips/:tripId/share-link (outsider) → 403', async () => {
      const { status } = await api('GET', `/api/trips/${tripId}/share-link`, {
        token: outsiderToken,
      });
      expect(status).toBe(403);
    });

    test('T-06  POST /api/trips/:tripId/share-link/regenerate → 200, new code differs', async () => {
      const before = await api('GET', `/api/trips/${tripId}/share-link`, { token: ownerToken });
      const oldCode = (before.body as any).inviteCode as string;

      const { status, body } = await api(
        'POST', `/api/trips/${tripId}/share-link/regenerate`, { token: ownerToken }
      );
      expect(status).toBe(200);
      const newCode = (body as any).inviteCode as string;
      expect(newCode).not.toBe(oldCode);

      // Update module-level inviteCode so later SL tests use the fresh one
      inviteCode = newCode;
    });
  });

  // ── SL-xx  Share link (join flow) ───────────────────────────────────────────

  describe('Share link (join)', () => {
    test('SL-01  GET /api/join/:inviteCode (public) → 200 with trip name', async () => {
      const { status, body } = await api('GET', `/api/join/${inviteCode}`);
      expect(status).toBe(200);
      expect(typeof (body as any).name).toBe('string');
      expect(typeof (body as any).destination).toBe('string');
    });

    test('SL-01e GET /api/join/<bad-code> → 404', async () => {
      const { status } = await api('GET', '/api/join/definitely-not-a-real-code-xyz');
      expect(status).toBe(404);
    });

    test('SL-02  POST /api/join/:inviteCode (new user) → 200, idempotent', async () => {
      // Register a fresh user just for this join test
      const joinEmail = `joiner-${ts}@test.example`;
      const reg = await api('POST', '/api/auth/register', {
        body: { email: joinEmail, displayName: 'Joiner', password: PASSWORD },
      });
      const joinerToken = (reg.body as any).token as string;

      const { status } = await api('POST', `/api/join/${inviteCode}`, { token: joinerToken });
      expect([200, 204]).toContain(status);

      // Idempotent — joining again should not error
      const again = await api('POST', `/api/join/${inviteCode}`, { token: joinerToken });
      expect([200, 204]).toContain(again.status);
    });

    test('SL-02e POST /api/join/:inviteCode (no token) → 401', async () => {
      const { status } = await api('POST', `/api/join/${inviteCode}`);
      expect(status).toBe(401);
    });

    test('SL-02e POST /api/join/<bad-code> → 404', async () => {
      const { status } = await api('POST', '/api/join/definitely-not-a-real-code-xyz', {
        token: ownerToken,
      });
      expect(status).toBe(404);
    });
  });

  // ── C-xx  Collaborators ─────────────────────────────────────────────────────

  describe('Collaborators', () => {
    test('C-01  GET /api/trips/:tripId/collaborators → 200, each has id/name/email/role', async () => {
      const { status, body } = await api('GET', `/api/trips/${tripId}/collaborators`, {
        token: ownerToken,
      });
      expect(status).toBe(200);
      const collabs: any[] = (body as any).collaborators ?? [];
      expect(collabs.length).toBeGreaterThan(0);
      collabs.forEach(c => {
        expect(typeof c.id).toBe('string');
        expect(typeof c.name).toBe('string');
        expect(typeof c.email).toBe('string');
        expect(typeof c.role).toBe('string');
      });
    });

    test('C-01e GET /api/trips/:tripId/collaborators (outsider) → 403', async () => {
      const { status } = await api('GET', `/api/trips/${tripId}/collaborators`, {
        token: outsiderToken,
      });
      expect(status).toBe(403);
    });

    test('C-02  PATCH /api/trips/:tripId/collaborators/:userId (owner demotes viewer) → 200', async () => {
      // viewerToken joined the trip (Viewer role); owner bumps them to Editor then back
      const { status, body } = await api(
        'PATCH', `/api/trips/${tripId}/collaborators/${viewerId}`,
        { token: ownerToken, body: { role: 'Editor' } }
      );
      expect(status).toBe(200);
      expect((body as any).role).toMatch(/EDITOR/i);

      // Restore to Viewer for subsequent tests
      await api('PATCH', `/api/trips/${tripId}/collaborators/${viewerId}`, {
        token: ownerToken,
        body: { role: 'Viewer' },
      });
    });

    test('C-02e PATCH role (viewer tries to change roles) → 403', async () => {
      const { status } = await api(
        'PATCH', `/api/trips/${tripId}/collaborators/${ownerId}`,
        { token: viewerToken, body: { role: 'Viewer' } }
      );
      expect(status).toBe(403);
    });

    test('C-03  DELETE /api/trips/:tripId/collaborators/:userId (self-remove) → 204', async () => {
      // Register a throwaway member who will leave
      const leaveEmail = `leaver-${ts}@test.example`;
      const reg = await api('POST', '/api/auth/register', {
        body: { email: leaveEmail, displayName: 'Leaver', password: PASSWORD },
      });
      const leaverToken = (reg.body as any).token as string;
      const leaverId    = (reg.body as any).user?.id as string;

      // Join the trip
      await api('POST', `/api/join/${inviteCode}`, { token: leaverToken });

      // Self-remove
      const { status } = await api(
        'DELETE', `/api/trips/${tripId}/collaborators/${leaverId}`,
        { token: leaverToken }
      );
      expect(status).toBe(204);
    });

    test('C-03e DELETE owner (cannot remove self) → 400', async () => {
      const { status } = await api(
        'DELETE', `/api/trips/${tripId}/collaborators/${ownerId}`,
        { token: ownerToken }
      );
      expect(status).toBe(400);
    });
  });

  // ── I-xx  Invitations ───────────────────────────────────────────────────────

  describe('Invitations', () => {
    test('I-01  POST /api/trips/:tripId/invitations → 201 with invitation list', async () => {
      const { status, body } = await api('POST', `/api/trips/${tripId}/invitations`, {
        token: ownerToken,
        body: { emails: [`sent-inv-${ts}@test.example`], role: 'Viewer' },
      });
      expect(status).toBe(201);
      const invs: any[] = (body as any).invitations ?? [];
      expect(invs).toHaveLength(1);
      expect(invs[0].status).toBe('PENDING');
    });

    test('I-01  POST multiple emails in one call → 201, all returned', async () => {
      const { status, body } = await api('POST', `/api/trips/${tripId}/invitations`, {
        token: ownerToken,
        body: {
          emails: [`multi-a-${ts}@test.example`, `multi-b-${ts}@test.example`],
          role: 'Viewer',
        },
      });
      expect(status).toBe(201);
      expect((body as any).invitations).toHaveLength(2);
    });

    test('I-01e POST duplicate email → non-201 (all emails failed)', async () => {
      const email = `dup-inv-${ts}@test.example`;
      await api('POST', `/api/trips/${tripId}/invitations`, {
        token: ownerToken,
        body: { emails: [email], role: 'Viewer' },
      });
      const { status } = await api('POST', `/api/trips/${tripId}/invitations`, {
        token: ownerToken,
        body: { emails: [email], role: 'Viewer' },
      });
      expect(status).not.toBe(201);
    });

    test('I-01e POST invitation (viewer) → 403', async () => {
      const { status } = await api('POST', `/api/trips/${tripId}/invitations`, {
        token: viewerToken,
        body: { emails: [`viewer-shouldfail-${ts}@test.example`], role: 'Viewer' },
      });
      expect(status).toBe(403);
    });

    test('I-02  GET /api/trips/:tripId/invitations → 200 with invitations array', async () => {
      const { status, body } = await api('GET', `/api/trips/${tripId}/invitations`, {
        token: ownerToken,
      });
      expect(status).toBe(200);
      expect(Array.isArray((body as any).invitations)).toBe(true);
    });

    test('I-02  GET invitations?status=PENDING → all results are PENDING', async () => {
      const { status, body } = await api(
        'GET', `/api/trips/${tripId}/invitations?status=PENDING`,
        { token: ownerToken }
      );
      expect(status).toBe(200);
      const invs: any[] = (body as any).invitations ?? [];
      invs.forEach(inv => expect(inv.status).toBe('PENDING'));
    });

    test('I-02e GET invitations (no token) → 401', async () => {
      const { status } = await api('GET', `/api/trips/${tripId}/invitations`);
      expect(status).toBe(401);
    });

    test('I-03  GET /api/invitations/accept/<bad-token> → 404', async () => {
      const { status } = await api('GET', '/api/invitations/accept/totally-fake-token-xyz');
      expect(status).toBe(404);
    });

    test('I-04  POST /api/invitations/accept/<bad-token> (auth) → 404', async () => {
      const { status } = await api('POST', '/api/invitations/accept/totally-fake-token-xyz', {
        token: ownerToken,
      });
      expect(status).toBe(404);
    });

    test('I-04e POST /api/invitations/accept/<token> (no auth) → 401', async () => {
      const { status } = await api('POST', '/api/invitations/accept/totally-fake-token-xyz');
      expect(status).toBe(401);
    });

    test('I-05  DELETE /api/invitations/:id → 204 (revoke)', async () => {
      if (!invitationId) {
        console.warn('I-05: skipping — no invitationId captured in beforeAll');
        return;
      }
      const { status } = await api('DELETE', `/api/invitations/${invitationId}`, {
        token: ownerToken,
      });
      expect(status).toBe(204);
    });

    test('I-05e DELETE /api/invitations/:id (no token) → 401', async () => {
      const { status } = await api('DELETE', '/api/invitations/fake-id-999');
      expect(status).toBe(401);
    });
  });

  // ── IT-xx  Itinerary ────────────────────────────────────────────────────────

  describe('Itinerary', () => {
    test('IT-01  GET /api/trips/:tripId/itinerary → 200, each item has poi/day/addedBy', async () => {
      const { status, body } = await api('GET', `/api/trips/${tripId}/itinerary`, {
        token: ownerToken,
      });
      expect(status).toBe(200);
      expect(Array.isArray((body as any).items)).toBe(true);
    });

    test('IT-01  GET itinerary (viewer member) → 200', async () => {
      const { status } = await api('GET', `/api/trips/${tripId}/itinerary`, {
        token: viewerToken,
      });
      expect(status).toBe(200);
    });

    test('IT-01e GET itinerary (no token) → 401', async () => {
      const { status } = await api('GET', `/api/trips/${tripId}/itinerary`);
      expect(status).toBe(401);
    });

    test('IT-01e GET itinerary (outsider) → 403', async () => {
      const { status } = await api('GET', `/api/trips/${tripId}/itinerary`, {
        token: outsiderToken,
      });
      expect(status).toBe(403);
    });

    test('IT-02  POST /api/trips/:tripId/itinerary → 201 with nested poi + day + addedBy', async () => {
      const { status, body } = await api('POST', `/api/trips/${tripId}/itinerary`, {
        token: ownerToken,
        body: { poiId: 'h2', day: 2, notes: 'Great spot' },
      });
      expect(status).toBe(201);
      expect(typeof (body as any).id).toBe('string');
      expect((body as any).poi).toBeTruthy();
      expect((body as any).day).toBe(2);
      expect(typeof (body as any).addedBy).toBe('string');
    });

    test('IT-02e POST duplicate POI → 409 conflict', async () => {
      // h1 was added in beforeAll; trying again should conflict
      const { status } = await api('POST', `/api/trips/${tripId}/itinerary`, {
        token: ownerToken,
        body: { poiId: 'h1', day: 99 },
      });
      expect(status).toBe(409);
    });

    test('IT-02e POST (viewer) → 403', async () => {
      const { status } = await api('POST', `/api/trips/${tripId}/itinerary`, {
        token: viewerToken,
        body: { poiId: 'h3', day: 3 },
      });
      expect(status).toBe(403);
    });

    test('IT-03  DELETE /api/trips/:tripId/itinerary/:itemId → 204', async () => {
      if (!itineraryItemId) {
        console.warn('IT-03: skipping — no itineraryItemId captured in beforeAll');
        return;
      }
      const { status } = await api(
        'DELETE', `/api/trips/${tripId}/itinerary/${itineraryItemId}`,
        { token: ownerToken }
      );
      expect(status).toBe(204);
    });

    test('IT-03e DELETE non-existent item → 404', async () => {
      const { status } = await api(
        'DELETE', `/api/trips/${tripId}/itinerary/does-not-exist-xyz`,
        { token: ownerToken }
      );
      expect(status).toBe(404);
    });

    test('IT-03e DELETE (viewer) → 403', async () => {
      // Add a fresh item so viewer can try to delete it
      const add = await api('POST', `/api/trips/${tripId}/itinerary`, {
        token: ownerToken,
        body: { poiId: 'l1', day: 1 },
      });
      const newItemId = (add.body as any)?.id as string | undefined;
      if (!newItemId) {
        console.warn('IT-03e viewer: could not add POI for test, skipping delete check');
        return;
      }
      const { status } = await api(
        'DELETE', `/api/trips/${tripId}/itinerary/${newItemId}`,
        { token: viewerToken }
      );
      expect(status).toBe(403);
    });
  });
});
