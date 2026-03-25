import { useRef } from 'react';
import { Users, Search, MapPin } from 'lucide-react';
import type { PanelTab, Collaborator, Role, POI, ItineraryItem } from '../types';
import { CollaboratorsPanel } from './CollaboratorsPanel';
import { POISearch } from './POISearch';
import { ItineraryPanel } from './ItineraryPanel';

interface Tab {
  id: PanelTab;
  label: string;
  Icon: React.ElementType;
  badge?: number;
}

interface RightPanelProps {
  activeTab: PanelTab;
  onTabChange: (tab: PanelTab) => void;
  collaborators: Collaborator[];
  onUpdateRole: (id: string, role: Role) => void;
  onRemoveCollaborator: (id: string) => void;
  itinerary: ItineraryItem[];
  onAddPOI: (poi: POI, day: number) => void;
  onRemoveItineraryItem: (id: string) => void;
  onDeleteDay: (day: number) => void;
  onReorderItinerary: (items: ItineraryItem[]) => void;
  onHoverPOI?: (poi: POI | null) => void;
  destination?: string;
}

export function RightPanel({
  activeTab,
  onTabChange,
  collaborators,
  onUpdateRole,
  onRemoveCollaborator,
  itinerary,
  onAddPOI,
  onRemoveItineraryItem,
  onDeleteDay,
  onReorderItinerary,
  onHoverPOI,
  destination,
}: RightPanelProps) {
  const tabs: Tab[] = [
    { id: 'collaborators', label: 'People',    Icon: Users,   badge: collaborators.length },
    { id: 'search',        label: 'Explore',   Icon: Search                               },
    { id: 'itinerary',     label: 'Itinerary', Icon: MapPin,  badge: itinerary.length     },
  ];

  const tabIds = tabs.map(t => t.id);
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([]);

  function handleTabKeyDown(e: React.KeyboardEvent) {
    const currentIndex = tabIds.indexOf(activeTab);
    let nextIndex = currentIndex;
    if (e.key === 'ArrowRight') {
      nextIndex = (currentIndex + 1) % tabs.length;
    } else if (e.key === 'ArrowLeft') {
      nextIndex = (currentIndex - 1 + tabs.length) % tabs.length;
    } else if (e.key === 'Home') {
      nextIndex = 0;
    } else if (e.key === 'End') {
      nextIndex = tabs.length - 1;
    } else {
      return;
    }
    e.preventDefault();
    onTabChange(tabIds[nextIndex]);
    tabRefs.current[nextIndex]?.focus();
  }

  return (
    <div className="flex flex-col h-full bg-white border-l border-gray-200 overflow-hidden">
      {/* Tab bar */}
      <nav
        className="flex border-b border-gray-200 flex-shrink-0"
        role="tablist"
        aria-label="Trip management tabs"
        onKeyDown={handleTabKeyDown}
      >
        {tabs.map(({ id, label, Icon, badge }, i) => (
          <button
            key={id}
            ref={el => { tabRefs.current[i] = el; }}
            role="tab"
            aria-selected={activeTab === id}
            aria-controls={`tabpanel-${id}`}
            id={`tab-${id}`}
            tabIndex={activeTab === id ? 0 : -1}
            onClick={() => onTabChange(id)}
            className={`flex-1 flex flex-col items-center gap-0.5 py-2 min-h-[44px] text-xs font-medium transition-colors border-b-2 relative ${
              activeTab === id
                ? 'border-blue-600 text-blue-600'
                : 'border-transparent text-gray-500 hover:text-gray-700 hover:bg-gray-50'
            }`}
          >
            <div className="relative">
              <Icon className="w-5 h-5" aria-hidden="true" />
              {badge !== undefined && badge > 0 && (
                <span
                  className={`absolute -top-1.5 -right-2 min-w-[16px] h-4 rounded-full text-[10px] font-bold flex items-center justify-center px-1 ${
                    activeTab === id ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-600'
                  }`}
                  aria-label={`${badge} ${label.toLowerCase()}`}
                >
                  {badge}
                </span>
              )}
            </div>
            <span className="hidden sm:inline">{label}</span>
          </button>
        ))}
      </nav>

      {/* Tab panel — single container so flex-1 gives it the full remaining height */}
      <div
        className="flex-1 overflow-hidden flex flex-col"
        role="tabpanel"
        id={`tabpanel-${activeTab}`}
        aria-labelledby={`tab-${activeTab}`}
      >
        {activeTab === 'collaborators' && (
          <CollaboratorsPanel
            collaborators={collaborators}
            onUpdateRole={onUpdateRole}
            onRemove={onRemoveCollaborator}
          />
        )}
        {activeTab === 'search' && (
          <POISearch
            itinerary={itinerary}
            onAddPOI={onAddPOI}
            onHoverPOI={onHoverPOI}
            near={destination}
          />
        )}
        {activeTab === 'itinerary' && (
          <ItineraryPanel
            itinerary={itinerary}
            onRemove={onRemoveItineraryItem}
            onDeleteDay={onDeleteDay}
            onReorder={onReorderItinerary}
          />
        )}
      </div>
    </div>
  );
}
