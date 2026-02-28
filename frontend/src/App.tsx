import { useState, useCallback, useEffect } from 'react';
import { Share2, Pencil, X, Check, SunMoon } from 'lucide-react';
import type { Trip, Collaborator, Role, POI, ItineraryItem, PanelTab, Toast } from './types';
import { mockTrip, mockItinerary } from './data/mockData';
import { MapView } from './components/MapView';
import { RightPanel } from './components/RightPanel';
import { InviteModal } from './components/InviteModal';
import { Avatar } from './components/Avatar';
import { ToastContainer } from './components/Toast';

// Deterministic avatar colours for newly-invited users
const INVITE_COLORS = [
  '#6366F1', '#EC4899', '#14B8A6', '#F97316',
  '#84CC16', '#EAB308', '#8B5CF6', '#06B6D4',
];

let colorIndex = 0;
function nextColor() {
  return INVITE_COLORS[colorIndex++ % INVITE_COLORS.length];
}

let toastSeq = 0;
function makeToast(message: string, type: Toast['type'] = 'success'): Toast {
  return { id: String(++toastSeq), message, type };
}

export default function App() {
  const [trip, setTrip]                 = useState<Trip>(mockTrip);
  const [itinerary, setItinerary]       = useState<ItineraryItem[]>(mockItinerary);
  const [activeTab, setActiveTab]       = useState<PanelTab>('collaborators');
  const [showInviteModal, setInvite]    = useState(false);
  const [editingName, setEditingName]   = useState(false);
  const [nameInput, setNameInput]       = useState(trip.name);
  const [toasts, setToasts]             = useState<Toast[]>([]);
  const [hoveredPOI, setHoveredPOI]     = useState<POI | null>(null);
  const [highContrast, setHighContrast] = useState(() => localStorage.getItem('hc') === '1');

  useEffect(() => {
    localStorage.setItem('hc', highContrast ? '1' : '0');
  }, [highContrast]);

  // ── Toast helpers ────────────────────────────────────────────────────────────
  const pushToast = useCallback((message: string, type: Toast['type'] = 'success') => {
    setToasts(prev => [...prev, makeToast(message, type)]);
  }, []);

  const dismissToast = useCallback((id: string) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  // ── Trip name ────────────────────────────────────────────────────────────────
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

  // ── Collaborators ────────────────────────────────────────────────────────────
  const handleSendInvites = useCallback((emails: string[], role: Extract<Role, 'Editor' | 'Viewer'>) => {
    const newCollabs: Collaborator[] = emails.map((email, i) => {
      // Derive a display name from the email local-part
      const local = email.split('@')[0];
      const name  = local
        .replace(/[._-]+/g, ' ')
        .replace(/\b\w/g, c => c.toUpperCase());
      return {
        id:    `inv-${Date.now()}-${i}`,
        name,
        email,
        role,
        color: nextColor(),
      };
    });
    setTrip(prev => ({ ...prev, collaborators: [...prev.collaborators, ...newCollabs] }));
    setActiveTab('collaborators');
    pushToast(
      emails.length === 1
        ? `Invite sent to ${emails[0]}`
        : `Invites sent to ${emails.length} people`,
    );
  }, [pushToast]);

  const handleUpdateRole = useCallback((id: string, role: Role) => {
    setTrip(prev => ({
      ...prev,
      collaborators: prev.collaborators.map(c => c.id === id ? { ...c, role } : c),
    }));
  }, []);

  const handleRemoveCollaborator = useCallback((id: string) => {
    setTrip(prev => ({
      ...prev,
      collaborators: prev.collaborators.filter(c => c.id !== id),
    }));
  }, []);

  // ── Itinerary ────────────────────────────────────────────────────────────────
  const handleAddPOI = useCallback((poi: POI, day: number) => {
    const item: ItineraryItem = {
      id:      `item-${Date.now()}`,
      poi,
      addedBy: trip.collaborators[0]?.name ?? 'You',
      day,
    };
    setItinerary(prev => [...prev, item]);
    pushToast(`Added "${poi.name}" to Day ${day}`);
  }, [trip.collaborators, pushToast]);

  const handleRemoveItem = useCallback((id: string) => {
    setItinerary(prev => prev.filter(i => i.id !== id));
  }, []);

  const handleReorderItinerary = useCallback((newItems: ItineraryItem[]) => {
    setItinerary(newItems);
  }, []);

  const handleDeleteDay = useCallback((day: number) => {
    setItinerary(prev =>
      prev
        .filter(i => i.day !== day)
        // Renumber: days above the deleted one shift down by 1
        .map(i => i.day > day ? { ...i, day: i.day - 1 } : i),
    );
    pushToast(`Day ${day} deleted`);
  }, [pushToast]);

  // ── Render ───────────────────────────────────────────────────────────────────
  return (
    <div className="flex flex-col h-screen bg-gray-50 overflow-hidden" data-hc={highContrast ? 'true' : undefined}>

      {/* ── Skip navigation ─────────────────────────────────────────────────── */}
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:px-4 focus:py-2 focus:bg-blue-600 focus:text-white focus:rounded-lg focus:text-sm focus:font-medium focus:shadow-lg"
      >
        Skip to main content
      </a>

      {/* ── Header ─────────────────────────────────────────────────────────── */}
      <header className="flex items-center justify-between gap-4 px-4 py-2.5 bg-white border-b border-gray-200 flex-shrink-0 z-10">

        {/* Trip name (editable) + high contrast toggle */}
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
              <button onClick={saveName}       aria-label="Save trip name"       className="p-1 rounded hover:bg-green-50 text-green-600 transition-colors"><Check className="w-4 h-4" /></button>
              <button onClick={cancelNameEdit} aria-label="Cancel name editing"  className="p-1 rounded hover:bg-red-50   text-red-400   transition-colors"><X     className="w-4 h-4" /></button>
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

          {/* High contrast toggle */}
          <button
            onClick={() => setHighContrast(v => !v)}
            aria-pressed={highContrast}
            className={`p-1.5 rounded-md transition-colors flex-shrink-0 ${
              highContrast
                ? 'bg-gray-900 text-white'
                : 'bg-gray-100 text-gray-500 hover:bg-gray-200'
            }`}
            aria-label="Toggle high contrast mode"
          >
            <SunMoon className="w-3.5 h-3.5" aria-hidden="true" />
          </button>
        </div>

        {/* Right side: collaborator avatars + Share button */}
        <div className="flex items-center gap-3 flex-shrink-0">
          {/* Avatars (hidden on very small screens) */}
          <div
            className="hidden sm:flex -space-x-2"
            aria-label={`${trip.collaborators.length} collaborators`}
          >
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

          {/* Share Trip */}
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

      {/* ── Main content ────────────────────────────────────────────────────── */}
      <main id="main-content" className="flex-1 flex flex-col md:flex-row overflow-hidden">

        {/* Map — full width on mobile (fixed height), fills remaining space on desktop */}
        <section
          className="h-48 md:h-auto md:flex-1 relative flex-shrink-0"
          aria-label="Trip map"
        >
          <MapView itinerary={itinerary} highlightPOI={hoveredPOI} />
        </section>

        {/* Right panel — full width below map on mobile, fixed sidebar on desktop */}
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
          />
        </aside>
      </main>

      {/* ── Invite modal ────────────────────────────────────────────────────── */}
      {showInviteModal && (
        <InviteModal
          tripName={trip.name}
          shareLink={trip.shareLink}
          onClose={() => setInvite(false)}
          onSendInvites={handleSendInvites}
        />
      )}

      {/* ── Toast notifications ──────────────────────────────────────────────── */}
      <ToastContainer toasts={toasts} onDismiss={dismissToast} />
    </div>
  );
}
