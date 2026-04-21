import { useState, useCallback, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { Share2, Pencil, X, Check, SunMoon, LogOut, MapPin, ChevronRight, Plus } from 'lucide-react';
import type { Trip, Collaborator, Role, POI, ItineraryItem, PanelTab, Toast } from './types';
import { MapView } from './components/MapView';
import { RightPanel } from './components/RightPanel';
import { InviteModal } from './components/InviteModal';
import { Avatar } from './components/Avatar';
import { ToastContainer } from './components/Toast';
import { api } from './api';

// Deterministic avatar colours for collaborators that arrive without one
const INVITE_COLORS = [
  '#6366F1', '#EC4899', '#14B8A6', '#F97316',
  '#84CC16', '#EAB308', '#8B5CF6', '#06B6D4',
];
let colorIndex = 0;
function nextColor() { return INVITE_COLORS[colorIndex++ % INVITE_COLORS.length]; }

let toastSeq = 0;
function makeToast(message: string, type: Toast['type'] = 'success'): Toast {
  return { id: String(++toastSeq), message, type };
}

export default function App() {
  const [trip, setTrip]                 = useState<Trip | null>(null);
  const [tripId, setTripId]             = useState<string | null>(null);
  const [itinerary, setItinerary]       = useState<ItineraryItem[]>([]);
  const [activeTab, setActiveTab]       = useState<PanelTab>('collaborators');
  const [showInviteModal, setInvite]    = useState(false);
  const [editingName, setEditingName]   = useState(false);
  const [nameInput, setNameInput]       = useState('');
  const [toasts, setToasts]             = useState<Toast[]>([]);
  const [hoveredPOI, setHoveredPOI]     = useState<POI | null>(null);
  const [highContrast, setHighContrast] = useState(() => localStorage.getItem('hc') === '1');
  const [currentUser, setCurrentUser]   = useState<{ displayName: string; email: string } | null>(null);
  // Trip selector / create form state
  const [myTrips, setMyTrips]                   = useState<{ id: string; name: string; destination: string }[]>([]);
  const [showTripSelector, setShowTripSelector] = useState(false);
  const [newTripName, setNewTripName]           = useState('');
  const [newTripDestination, setNewTripDestination] = useState('');
  const [creatingTrip, setCreatingTrip]         = useState(false);
  const [showCreateForm, setShowCreateForm]     = useState(false);
  const wsRef    = useRef<WebSocket | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    localStorage.setItem('hc', highContrast ? '1' : '0');
  }, [highContrast]);

  // ── Backend bootstrap ─────────────────────────────────────────────────────
  useEffect(() => {
    let cancelled = false;
    async function init() {
      try {
        const { tripId: id } = await api.bootstrap();
        if (cancelled) return;

        // Fetch user info independently so it always shows even if trip load fails.
        api.getCurrentUser()
          .then(user => { if (!cancelled) setCurrentUser({ displayName: user.displayName, email: user.email }); })
          .catch(() => {});

        if (!id) {
          // No cached trip — fetch the list so the selector can show existing trips.
          try {
            const trips = await api.listTrips();
            if (!cancelled) setMyTrips(trips);
          } catch { /* show empty selector */ }
          setShowTripSelector(true);
          return;
        }

        setTripId(id);

        const [tripData, itineraryData] = await Promise.all([
          api.getTrip(id),
          api.getItinerary(id),
        ]);
        if (cancelled) return;
        setTrip(tripData);
        setItinerary(itineraryData);
        setNameInput(tripData.name);

        // ── WebSocket for real-time collaboration ─────────────────────────
        const BACKOFF = [2000, 4000, 8000, 16000, 32000];
        let attempts = 0;

        function connectWS(tripId: string) {
          const ws = api.createWSConnection(tripId);
          wsRef.current = ws;

          ws.onopen = () => { attempts = 0; };

          ws.onmessage = (evt) => {
            const lines = (evt.data as string).split('\n').filter(Boolean);
            for (const line of lines) {
              try {
                const { event, data } = JSON.parse(line) as { event: string; data: Record<string, unknown> };
                handleWSEvent(event, data);
              } catch { /* ignore malformed frames */ }
            }
          };

          ws.onerror = () => pushToast('Real-time connection error', 'error');

          ws.onclose = () => {
            if (cancelled) return;
            if (attempts < BACKOFF.length) {
              setTimeout(() => { if (!cancelled) { attempts++; connectWS(tripId); } }, BACKOFF[attempts]);
            }
          };
        }

        connectWS(id);
      } catch {
        // Not authenticated — the ProtectedRoute will redirect to /login.
        navigate('/login');
      }
    }
    init();

    function handleUnload() { wsRef.current?.close(); }
    window.addEventListener('beforeunload', handleUnload);

    return () => {
      cancelled = true;
      window.removeEventListener('beforeunload', handleUnload);
      wsRef.current?.close();
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function handleWSEvent(event: string, data: Record<string, unknown>) {
    switch (event) {
      case 'presence_update': {
        const online = (data.onlineUserIds as string[]) ?? [];
        setTrip(prev => prev ? ({
          ...prev,
          collaborators: prev.collaborators.map(c => ({
            ...c,
            // isOnline is not in the Collaborator type but harmless to spread
          })),
        }) : prev);
        // Re-fetch collaborators so online badges refresh
        if (tripId) api.getTrip(tripId).then(t => setTrip(t)).catch(() => {});
        void online; // used via re-fetch above
        break;
      }
      case 'collaborator_joined': {
        if (tripId) api.getTrip(tripId).then(t => setTrip(t)).catch(() => {});
        break;
      }
      case 'collaborator_left': {
        const userId = data.userId as string;
        setTrip(prev => prev ? ({
          ...prev,
          collaborators: prev.collaborators.filter(c => c.id !== userId),
        }) : null);
        break;
      }
      case 'role_updated': {
        const userId  = data.userId  as string;
        const newRole = data.newRole as Role;
        setTrip(prev => prev ? ({
          ...prev,
          collaborators: prev.collaborators.map(c => c.id === userId ? { ...c, role: newRole } : c),
        }) : null);
        break;
      }
      case 'itinerary_updated': {
        const action = data.action as string;
        if (action === 'added') {
          const item = data.item as ItineraryItem;
          setItinerary(prev => {
            if (prev.some(i => i.id === item.id)) return prev;
            return [...prev, item];
          });
        } else if (action === 'removed') {
          const itemId = data.itemId as string;
          setItinerary(prev => prev.filter(i => i.id !== itemId));
        }
        break;
      }
    }
  }

  // ── Toast helpers ─────────────────────────────────────────────────────────
  const pushToast = useCallback((message: string, type: Toast['type'] = 'success') => {
    setToasts(prev => [...prev, makeToast(message, type)]);
  }, []);

  const dismissToast = useCallback((id: string) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  // ── Auth ──────────────────────────────────────────────────────────────────
  function handleLogout() {
    wsRef.current?.close();
    api.logout();
    navigate('/login');
  }

  // ── Trip selector ─────────────────────────────────────────────────────────
  async function handleOpenTripSelector() {
    wsRef.current?.close();
    setTrip(null);
    setTripId(null);
    setItinerary([]);
    setNewTripName('');
    setNewTripDestination('');
    setShowCreateForm(false);
    try {
      const trips = await api.listTrips();
      setMyTrips(trips);
    } catch { /* keep stale list */ }
    setShowTripSelector(true);
  }

  async function handleSelectTrip(id: string) {
    api.selectTrip(id);
    setTripId(id);
    setShowTripSelector(false);
    const [tripData, itineraryData] = await Promise.all([
      api.getTrip(id),
      api.getItinerary(id),
    ]);
    setTrip(tripData);
    setItinerary(itineraryData);
    setNameInput(tripData.name);
  }

  // ── Create trip ───────────────────────────────────────────────────────────
  async function handleCreateTrip(e: React.FormEvent) {
    e.preventDefault();
    const name = newTripName.trim();
    const destination = newTripDestination.trim();
    if (!name || !destination) return;
    setCreatingTrip(true);
    try {
      const { id } = await api.createTrip(name, destination);
      setShowTripSelector(false);
      setTripId(id);
      const [tripData, itineraryData] = await Promise.all([
        api.getTrip(id),
        api.getItinerary(id),
      ]);
      setTrip(tripData);
      setItinerary(itineraryData);
      setNameInput(tripData.name);
    } catch (err) {
      pushToast((err as Error).message ?? 'Failed to create trip', 'error');
    } finally {
      setCreatingTrip(false);
    }
  }

  // ── Trip name ─────────────────────────────────────────────────────────────
  async function saveName() {
    const trimmed = nameInput.trim();
    if (trimmed && trip && trimmed !== trip.name) {
      setTrip(prev => prev ? { ...prev, name: trimmed } : prev);
      try {
        await api.updateTrip(tripId!, { name: trimmed });
      } catch (err) {
        // Revert optimistic update on failure
        setTrip(prev => prev ? { ...prev, name: trip.name } : prev);
        pushToast((err as Error).message ?? 'Failed to save trip name', 'error');
      }
    }
    setEditingName(false);
  }
  function cancelNameEdit() {
    setNameInput(trip?.name ?? '');
    setEditingName(false);
  }

  // ── Collaborators ─────────────────────────────────────────────────────────
  const handleSendInvites = useCallback(async (emails: string[], role: Extract<Role, 'Editor' | 'Viewer'>) => {
    if (!tripId) return;
    try {
      await api.sendInvites(tripId, emails, role);
      // Optimistically add placeholders until the WS collaborator_joined arrives.
      const newCollabs: Collaborator[] = emails.map((email, i) => {
        const local = email.split('@')[0];
        const name  = local.replace(/[._-]+/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
        return { id: `pending-${Date.now()}-${i}`, name, email, role, color: nextColor() };
      });
      setTrip(prev => prev ? ({ ...prev, collaborators: [...prev.collaborators, ...newCollabs] }) : null);
      setActiveTab('collaborators');
      pushToast(
        emails.length === 1 ? `Invite sent to ${emails[0]}` : `Invites sent to ${emails.length} people`,
      );
    } catch (err) {
      pushToast((err as Error).message ?? 'Failed to send invites', 'error');
    }
  }, [tripId, pushToast]);

  const handleUpdateRole = useCallback(async (id: string, role: Role) => {
    if (!tripId) return;
    try {
      await api.updateCollaboratorRole(tripId, id, role);
      setTrip(prev => prev ? ({
        ...prev,
        collaborators: prev.collaborators.map(c => c.id === id ? { ...c, role } : c),
      }) : null);
    } catch (err) {
      pushToast((err as Error).message ?? 'Failed to update role', 'error');
    }
  }, [tripId, pushToast]);

  const handleRemoveCollaborator = useCallback(async (id: string) => {
    if (!tripId) return;
    try {
      await api.removeCollaborator(tripId, id);
      setTrip(prev => prev ? ({
        ...prev,
        collaborators: prev.collaborators.filter(c => c.id !== id),
      }) : null);
    } catch (err) {
      pushToast((err as Error).message ?? 'Failed to remove collaborator', 'error');
    }
  }, [tripId, pushToast]);

  // ── Itinerary ─────────────────────────────────────────────────────────────
  const handleAddPOI = useCallback(async (poi: POI, day: number) => {
    if (!tripId) return;
    try {
      const item = await api.addToItinerary(tripId, poi.id, day);
      // The WS itinerary_updated event also arrives; de-dup by ID in the handler.
      setItinerary(prev => prev.some(i => i.id === item.id) ? prev : [...prev, item]);
      pushToast(`Added "${poi.name}" to Day ${day}`);
    } catch (err) {
      const msg = (err as Error).message ?? '';
      if (msg.includes('already on the itinerary')) {
        pushToast(`"${poi.name}" is already on the itinerary`, 'info');
      } else {
        pushToast(msg || 'Failed to add to itinerary', 'error');
      }
    }
  }, [tripId, pushToast]);

  const handleRemoveItem = useCallback(async (id: string) => {
    if (!tripId) return;
    try {
      await api.removeFromItinerary(tripId, id);
      setItinerary(prev => prev.filter(i => i.id !== id));
    } catch (err) {
      pushToast((err as Error).message ?? 'Failed to remove item', 'error');
    }
  }, [tripId, pushToast]);

  const handleReorderItinerary = useCallback((newItems: ItineraryItem[]) => {
    setItinerary(newItems);
  }, []);

  const handleDeleteDay = useCallback((day: number) => {
    setItinerary(prev =>
      prev
        .filter(i => i.day !== day)
        .map(i => i.day > day ? { ...i, day: i.day - 1 } : i),
    );
    pushToast(`Day ${day} deleted`);
  }, [pushToast]);

  // ── Render ────────────────────────────────────────────────────────────────

  // ── Trip selector / Create trip screen ───────────────────────────────────
  if (showTripSelector) {
    const hasTrips = myTrips.length > 0;
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center p-4">
        <div className="bg-white rounded-2xl shadow-lg p-8 w-full max-w-md">

          {/* Header */}
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 bg-blue-50 rounded-xl">
              <MapPin className="w-6 h-6 text-blue-600" aria-hidden="true" />
            </div>
            <div>
              <h1 className="text-xl font-semibold text-gray-900">
                {hasTrips ? 'Your trips' : 'Create your first trip'}
              </h1>
              {currentUser && (
                <p className="text-sm text-gray-500">{currentUser.displayName}</p>
              )}
            </div>
          </div>

          {/* Existing trips list */}
          {hasTrips && !showCreateForm && (
            <ul className="space-y-2 mb-4">
              {myTrips.map(t => (
                <li key={t.id}>
                  <button
                    onClick={() => handleSelectTrip(t.id)}
                    className="w-full flex items-center justify-between px-4 py-3 rounded-xl border border-gray-200 hover:border-blue-400 hover:bg-blue-50 transition-colors text-left group"
                  >
                    <div>
                      <p className="text-sm font-medium text-gray-900 group-hover:text-blue-700">{t.name}</p>
                      <p className="text-xs text-gray-400">{t.destination}</p>
                    </div>
                    <ChevronRight className="w-4 h-4 text-gray-300 group-hover:text-blue-500 flex-shrink-0" />
                  </button>
                </li>
              ))}
            </ul>
          )}

          {/* Create new trip button (when trips exist and form is hidden) */}
          {hasTrips && !showCreateForm && (
            <button
              onClick={() => setShowCreateForm(true)}
              className="w-full flex items-center justify-center gap-2 py-2.5 px-4 border-2 border-dashed border-gray-300 text-gray-500 rounded-xl text-sm font-medium hover:border-blue-400 hover:text-blue-600 transition-colors mb-4"
            >
              <Plus className="w-4 h-4" />
              New trip
            </button>
          )}

          {/* Create trip form */}
          {(!hasTrips || showCreateForm) && (
            <form onSubmit={handleCreateTrip} className="space-y-4 mb-4">
              <div>
                <label htmlFor="trip-name" className="block text-sm font-medium text-gray-700 mb-1">
                  Trip name
                </label>
                <input
                  id="trip-name"
                  type="text"
                  value={newTripName}
                  onChange={e => setNewTripName(e.target.value)}
                  placeholder="e.g. Tokyo Adventure"
                  required
                  autoFocus
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
              </div>
              <div>
                <label htmlFor="trip-destination" className="block text-sm font-medium text-gray-700 mb-1">
                  Destination
                </label>
                <input
                  id="trip-destination"
                  type="text"
                  value={newTripDestination}
                  onChange={e => setNewTripDestination(e.target.value)}
                  placeholder="e.g. Tokyo, Japan"
                  required
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
              </div>
              <div className="flex gap-2">
                {showCreateForm && (
                  <button
                    type="button"
                    onClick={() => setShowCreateForm(false)}
                    className="flex-1 py-2.5 px-4 border border-gray-300 text-gray-600 rounded-lg text-sm font-medium hover:bg-gray-50 transition-colors"
                  >
                    Cancel
                  </button>
                )}
                <button
                  type="submit"
                  disabled={creatingTrip || !newTripName.trim() || !newTripDestination.trim()}
                  className="flex-1 py-2.5 px-4 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                >
                  {creatingTrip ? 'Creating…' : 'Create trip'}
                </button>
              </div>
            </form>
          )}

          <button
            onClick={() => { api.logout(); navigate('/login'); }}
            className="w-full text-center text-sm text-gray-400 hover:text-gray-600 transition-colors"
          >
            Log out
          </button>
        </div>
        <ToastContainer toasts={toasts} onDismiss={dismissToast} />
      </div>
    );
  }

  // Trip is loading (tripId was set but trip data not yet fetched).
  if (!trip) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <p className="text-gray-500 text-sm">Loading trip…</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-screen bg-gray-50 overflow-hidden" data-hc={highContrast ? 'true' : undefined}>

      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:px-4 focus:py-2 focus:bg-blue-600 focus:text-white focus:rounded-lg focus:text-sm focus:font-medium focus:shadow-lg"
      >
        Skip to main content
      </a>

      {/* ── Header ──────────────────────────────────────────────────────── */}
      <header className="flex items-center justify-between gap-4 px-4 py-2 md:py-2.5 bg-white border-b border-gray-200 flex-shrink-0 z-10">
        <div className="flex items-center gap-2 min-w-0">
          {editingName ? (
            <div className="flex items-center gap-1.5">
              <input
                type="text"
                value={nameInput}
                onChange={e => setNameInput(e.target.value)}
                onKeyDown={e => {
                  if (e.key === 'Enter')  saveName();
                  if (e.key === 'Escape') cancelNameEdit();
                }}
                className="text-base font-semibold text-gray-900 border-b-2 border-blue-500 bg-transparent focus:outline-none px-0.5 min-w-0 max-w-[220px]"
                aria-label="Edit trip name"
                autoFocus
              />
              <button onClick={saveName}       aria-label="Save trip name"      className="p-1 rounded hover:bg-green-50 text-green-600 transition-colors"><Check className="w-4 h-4" /></button>
              <button onClick={cancelNameEdit} aria-label="Cancel name editing" className="p-1 rounded hover:bg-red-50   text-red-400   transition-colors"><X     className="w-4 h-4" /></button>
            </div>
          ) : (
            <div className="flex items-center gap-1.5 min-w-0">
              <h1 className="text-base font-semibold text-gray-900 truncate">{trip.name}</h1>
              <button
                onClick={() => { setNameInput(trip.name); setEditingName(true); }}
                className="p-1 rounded-md hover:bg-gray-100 transition-colors flex-shrink-0"
                aria-label="Edit trip name"
              >
                <Pencil className="w-3.5 h-3.5 text-gray-400" />
              </button>
            </div>
          )}
          <button
            onClick={() => setHighContrast(v => !v)}
            aria-pressed={highContrast}
            className={`p-1.5 rounded-md transition-colors flex-shrink-0 ${
              highContrast ? 'bg-gray-900 text-white' : 'bg-gray-100 text-gray-500 hover:bg-gray-200'
            }`}
            aria-label="Toggle high contrast mode"
          >
            <SunMoon className="w-3.5 h-3.5" aria-hidden="true" />
          </button>
        </div>

        <div className="flex items-center gap-3 flex-shrink-0">
          <div className="hidden sm:flex -space-x-2" aria-label={`${trip.collaborators.length} collaborators`}>
            {trip.collaborators.slice(0, 5).map(c => (
              <Avatar key={c.id} name={c.name} color={c.color} size="sm" avatarUrl={c.avatarUrl} />
            ))}
            {trip.collaborators.length > 5 && (
              <div
                className="w-8 h-8 rounded-full bg-gray-200 ring-2 ring-white flex items-center justify-center text-xs text-gray-600 font-semibold"
                aria-label={`and ${trip.collaborators.length - 5} more collaborators`}
              >
                +{trip.collaborators.length - 5}
              </div>
            )}
          </div>
          <button
            onClick={() => setInvite(true)}
            className="flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 text-white text-sm font-medium hover:bg-blue-700 active:bg-blue-800 transition-colors"
            aria-label="Share trip and invite collaborators"
          >
            <Share2 className="w-4 h-4" aria-hidden="true" />
            <span>Share Trip</span>
          </button>

          <div className="flex items-center gap-2 border-l border-gray-200 pl-3">
            {currentUser && (
              <span className="hidden sm:block text-sm text-gray-600 truncate max-w-[120px]" title={currentUser.email}>
                {currentUser.displayName}
              </span>
            )}
            <button
              onClick={handleOpenTripSelector}
              className="p-1.5 rounded-md text-gray-400 hover:text-gray-700 hover:bg-gray-100 transition-colors"
              aria-label="My trips"
              title="My trips"
            >
              <MapPin className="w-4 h-4" aria-hidden="true" />
            </button>
            <button
              onClick={handleLogout}
              className="p-1.5 rounded-md text-gray-400 hover:text-gray-700 hover:bg-gray-100 transition-colors"
              aria-label="Log out"
              title="Log out"
            >
              <LogOut className="w-4 h-4" aria-hidden="true" />
            </button>
          </div>
        </div>
      </header>

      {/* ── Main content ────────────────────────────────────────────────── */}
      <main id="main-content" className="flex-1 flex flex-col md:flex-row overflow-hidden">
        <section
          className="h-[50%] min-h-[200px] md:h-auto md:flex-1 relative flex-shrink-0"
          aria-label="Trip map"
        >
          <MapView itinerary={itinerary} highlightPOI={hoveredPOI} />
        </section>
        <aside
          className="flex-1 md:flex-none md:w-96 overflow-hidden flex flex-col border-t md:border-t-0 border-gray-200"
          aria-label="Trip management"
        >
          <RightPanel
            activeTab={activeTab}
            onTabChange={setActiveTab}
            collaborators={trip.collaborators}
            onUpdateRole={handleUpdateRole}
            onRemoveCollaborator={handleRemoveCollaborator}
            itinerary={itinerary}
            onAddPOI={handleAddPOI}
            onRemoveItineraryItem={handleRemoveItem}
            onDeleteDay={handleDeleteDay}
            onReorderItinerary={handleReorderItinerary}
            onHoverPOI={setHoveredPOI}
            destination={trip.destination}
          />
        </aside>
      </main>

      {/* ── Invite modal ─────────────────────────────────────────────────── */}
      {showInviteModal && (
        <InviteModal
          tripName={trip.name}
          shareLink={trip.shareLink}
          onClose={() => setInvite(false)}
          onSendInvites={handleSendInvites}
        />
      )}

      {/* ── Toast notifications ──────────────────────────────────────────── */}
      <ToastContainer toasts={toasts} onDismiss={dismissToast} />
    </div>
  );
}
