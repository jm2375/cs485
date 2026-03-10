import { useState, useCallback, useEffect, useRef } from 'react';
import { Share2, Pencil, X, Check, SunMoon } from 'lucide-react';
import type { Trip, Collaborator, Role, POI, ItineraryItem, PanelTab, Toast } from './types';
import { mockTrip, mockItinerary } from './data/mockData';
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
  const [trip, setTrip]                 = useState<Trip>(mockTrip);
  const [tripId, setTripId]             = useState<string>(mockTrip.id);
  const [itinerary, setItinerary]       = useState<ItineraryItem[]>(mockItinerary);
  const [activeTab, setActiveTab]       = useState<PanelTab>('collaborators');
  const [showInviteModal, setInvite]    = useState(false);
  const [editingName, setEditingName]   = useState(false);
  const [nameInput, setNameInput]       = useState(trip.name);
  const [toasts, setToasts]             = useState<Toast[]>([]);
  const [hoveredPOI, setHoveredPOI]     = useState<POI | null>(null);
  const [highContrast, setHighContrast] = useState(() => localStorage.getItem('hc') === '1');
  const wsRef = useRef<WebSocket | null>(null);

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
        const ws = api.createWSConnection(id);
        wsRef.current = ws;

        ws.onmessage = (evt) => {
          // The server may batch multiple newline-separated JSON messages.
          const lines = (evt.data as string).split('\n').filter(Boolean);
          for (const line of lines) {
            try {
              const { event, data } = JSON.parse(line) as { event: string; data: Record<string, unknown> };
              handleWSEvent(event, data);
            } catch { /* ignore malformed frames */ }
          }
        };
        ws.onerror = () => pushToast('Real-time connection error', 'error');
      } catch (err) {
        console.warn('[App] backend not reachable — using mock data', err);
      }
    }
    init();
    return () => {
      cancelled = true;
      wsRef.current?.close();
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function handleWSEvent(event: string, data: Record<string, unknown>) {
    switch (event) {
      case 'presence_update': {
        const online = (data.onlineUserIds as string[]) ?? [];
        setTrip(prev => ({
          ...prev,
          collaborators: prev.collaborators.map(c => ({
            ...c,
            // isOnline is not in the Collaborator type but harmless to spread
          })),
        }));
        // Re-fetch collaborators so online badges refresh
        api.getTrip(tripId)
          .then(t => setTrip(t))
          .catch(() => {});
        void online; // used via re-fetch above
        break;
      }
      case 'collaborator_joined': {
        api.getTrip(tripId).then(t => setTrip(t)).catch(() => {});
        break;
      }
      case 'collaborator_left': {
        const userId = data.userId as string;
        setTrip(prev => ({
          ...prev,
          collaborators: prev.collaborators.filter(c => c.id !== userId),
        }));
        break;
      }
      case 'role_updated': {
        const userId  = data.userId  as string;
        const newRole = data.newRole as Role;
        setTrip(prev => ({
          ...prev,
          collaborators: prev.collaborators.map(c => c.id === userId ? { ...c, role: newRole } : c),
        }));
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

  // ── Trip name ─────────────────────────────────────────────────────────────
  function saveName() {
    const trimmed = nameInput.trim();
    if (trimmed && trimmed !== trip.name) {
      setTrip(prev => ({ ...prev, name: trimmed }));
    }
    setEditingName(false);
  }
  function cancelNameEdit() {
    setNameInput(trip.name);
    setEditingName(false);
  }

  // ── Collaborators ─────────────────────────────────────────────────────────
  const handleSendInvites = useCallback(async (emails: string[], role: Extract<Role, 'Editor' | 'Viewer'>) => {
    try {
      await api.sendInvites(tripId, emails, role);
      // Optimistically add placeholders until the WS collaborator_joined arrives.
      const newCollabs: Collaborator[] = emails.map((email, i) => {
        const local = email.split('@')[0];
        const name  = local.replace(/[._-]+/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
        return { id: `pending-${Date.now()}-${i}`, name, email, role, color: nextColor() };
      });
      setTrip(prev => ({ ...prev, collaborators: [...prev.collaborators, ...newCollabs] }));
      setActiveTab('collaborators');
      pushToast(
        emails.length === 1 ? `Invite sent to ${emails[0]}` : `Invites sent to ${emails.length} people`,
      );
    } catch (err) {
      pushToast((err as Error).message ?? 'Failed to send invites', 'error');
    }
  }, [tripId, pushToast]);

  const handleUpdateRole = useCallback(async (id: string, role: Role) => {
    try {
      await api.updateCollaboratorRole(tripId, id, role);
      setTrip(prev => ({
        ...prev,
        collaborators: prev.collaborators.map(c => c.id === id ? { ...c, role } : c),
      }));
    } catch (err) {
      pushToast((err as Error).message ?? 'Failed to update role', 'error');
    }
  }, [tripId, pushToast]);

  const handleRemoveCollaborator = useCallback(async (id: string) => {
    try {
      await api.removeCollaborator(tripId, id);
      setTrip(prev => ({
        ...prev,
        collaborators: prev.collaborators.filter(c => c.id !== id),
      }));
    } catch (err) {
      pushToast((err as Error).message ?? 'Failed to remove collaborator', 'error');
    }
  }, [tripId, pushToast]);

  // ── Itinerary ─────────────────────────────────────────────────────────────
  const handleAddPOI = useCallback(async (poi: POI, day: number) => {
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
