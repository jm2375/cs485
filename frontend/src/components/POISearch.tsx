import { useState, useMemo } from 'react';
import { Search, Star, MapPin, Plus, Check, ChevronDown, X } from 'lucide-react';
import type { POI, POICategory, ItineraryItem } from '../types';
import { mockPOIs } from '../data/mockData';

type CategoryFilter = POICategory | 'all';

const CATEGORY_TABS: { id: CategoryFilter; label: string; emoji: string }[] = [
  { id: 'all',         label: 'All',         emoji: '🗺️' },
  { id: 'restaurant',  label: 'Restaurants', emoji: '🍽️' },
  { id: 'landmark',    label: 'Landmarks',   emoji: '🏛️' },
  { id: 'hotel',       label: 'Hotels',      emoji: '🏨' },
  { id: 'attraction',  label: 'Attractions', emoji: '🎡' },
];

function priceLabel(level: number) {
  return '$'.repeat(level);
}

function priceAriaLabel(level: number) {
  return (['Free', 'Inexpensive', 'Moderate', 'Expensive', 'Very Expensive'] as const)[level] ?? '';
}

function StarRating({ rating }: { rating: number }) {
  return (
    <span className="flex items-center gap-0.5" aria-label={`Rating: ${rating} out of 5`}>
      <Star className="w-3 h-3 fill-amber-400 text-amber-400" aria-hidden="true" />
      <span className="text-xs font-medium text-gray-700">{rating.toFixed(1)}</span>
    </span>
  );
}

interface POISearchProps {
  itinerary: ItineraryItem[];
  onAddPOI: (poi: POI, day: number) => void;
  onHoverPOI?: (poi: POI | null) => void;
}

export function POISearch({ itinerary, onAddPOI, onHoverPOI }: POISearchProps) {
  const [query, setQuery]           = useState('');
  const [category, setCategory]     = useState<CategoryFilter>('all');
  const [pickingFor, setPickingFor] = useState<string | null>(null); // poi.id

  const addedIds = useMemo(
    () => new Set(itinerary.map(i => i.poi.id)),
    [itinerary],
  );

  // Sorted unique days already in the itinerary
  const existingDays = useMemo(
    () => [...new Set(itinerary.map(i => i.day))].sort((a, b) => a - b),
    [itinerary],
  );
  const nextNewDay = existingDays.length > 0 ? Math.max(...existingDays) + 1 : 1;

  const results = useMemo(() => {
    const q = query.toLowerCase();
    return mockPOIs.filter(poi => {
      const matchesCat = category === 'all' || poi.category === category;
      const matchesQ   =
        !q ||
        poi.name.toLowerCase().includes(q) ||
        poi.subcategory.toLowerCase().includes(q) ||
        poi.address.toLowerCase().includes(q) ||
        poi.description.toLowerCase().includes(q);
      return matchesCat && matchesQ;
    });
  }, [query, category]);

  function handleAddToDay(poi: POI, day: number) {
    onAddPOI(poi, day);
    setPickingFor(null);
  }

  return (
    <div className="flex flex-col h-full">
      {/* Search box */}
      <div className="px-3 pt-3 pb-2">
        <div className="relative">
          <Search
            className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none"
            aria-hidden="true"
          />
          <input
            type="search"
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Search restaurants, landmarks, hotels…"
            aria-label="Search for places to add to your itinerary"
            className="w-full pl-9 pr-4 py-2 rounded-lg border border-gray-200 bg-gray-50 text-sm text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition-shadow"
          />
        </div>
      </div>

      {/* Category filter tabs */}
      <div
        className="flex gap-1.5 px-3 pb-2.5 overflow-x-auto scrollbar-hide"
        role="tablist"
        aria-label="Filter by place category"
      >
        {CATEGORY_TABS.map(tab => (
          <button
            key={tab.id}
            role="tab"
            aria-selected={category === tab.id}
            onClick={() => setCategory(tab.id)}
            className={`flex items-center gap-1 px-3 py-1.5 rounded-full text-xs font-medium whitespace-nowrap transition-all flex-shrink-0 ${
              category === tab.id
                ? 'bg-blue-600 text-white shadow-sm'
                : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
            }`}
          >
            <span aria-hidden="true">{tab.emoji}</span>
            {tab.label}
          </button>
        ))}
      </div>

      {/* Results count */}
      <p className="px-3 pb-1 text-[11px] text-gray-400">
        {results.length} place{results.length !== 1 ? 's' : ''} found
      </p>

      {/* Results list */}
      <div className="flex-1 overflow-y-auto border-t border-gray-100" role="tabpanel">
        {results.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-40 gap-2 text-gray-400">
            <MapPin className="w-8 h-8" aria-hidden="true" />
            <p className="text-sm">No places found</p>
          </div>
        ) : (
          <ul aria-label="Search results">
            {results.map(poi => {
              const added      = addedIds.has(poi.id);
              const isPicking  = pickingFor === poi.id;

              return (
                <li
                  key={poi.id}
                  className="flex gap-3 p-3 hover:bg-gray-50 transition-colors border-b border-gray-50 last:border-b-0"
                  onMouseEnter={() => onHoverPOI?.(poi)}
                  onMouseLeave={() => onHoverPOI?.(null)}
                >
                  <img
                    src={poi.imageUrl}
                    alt={`Photo of ${poi.name}`}
                    className="w-20 h-16 rounded-lg object-cover flex-shrink-0 bg-gray-100"
                    loading="lazy"
                    width={80}
                    height={64}
                  />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold text-gray-900 truncate">{poi.name}</p>
                    <p className="text-[11px] text-gray-500 truncate">{poi.subcategory}</p>
                    <div className="flex items-center gap-2 mt-0.5">
                      <StarRating rating={poi.rating} />
                      <span className="text-[11px] text-gray-400">
                        ({poi.reviewCount.toLocaleString()})
                      </span>
                      <span className="text-[11px] text-gray-400" aria-label={priceAriaLabel(poi.priceLevel)}>{priceLabel(poi.priceLevel)}</span>
                    </div>
                    <p className="text-[11px] text-gray-400 truncate mt-0.5">{poi.address}</p>

                    {/* Add / day-picker */}
                    {added ? (
                      <span className="mt-1.5 inline-flex items-center gap-1 text-[11px] font-semibold px-2.5 py-1 rounded-full bg-green-50 text-green-700">
                        <Check className="w-3 h-3" aria-hidden="true" /> Added
                      </span>
                    ) : isPicking ? (
                      /* Inline day picker */
                      <div className="mt-2 flex flex-wrap items-center gap-1.5" role="group" aria-label={`Choose day for ${poi.name}`}>
                        <span className="text-[11px] text-gray-500 font-medium mr-0.5">Add to:</span>
                        {existingDays.map(day => (
                          <button
                            key={day}
                            onClick={() => handleAddToDay(poi, day)}
                            className="text-[11px] font-semibold px-2 py-0.5 rounded-full bg-blue-600 text-white hover:bg-blue-700 transition-colors"
                            aria-label={`Add ${poi.name} to Day ${day}`}
                          >
                            Day {day}
                          </button>
                        ))}
                        <button
                          onClick={() => handleAddToDay(poi, nextNewDay)}
                          className="text-[11px] font-semibold px-2 py-0.5 rounded-full bg-gray-800 text-white hover:bg-gray-900 transition-colors flex items-center gap-0.5"
                          aria-label={`Add ${poi.name} to a new day`}
                        >
                          <Plus className="w-2.5 h-2.5" aria-hidden="true" />
                          New Day
                        </button>
                        <button
                          onClick={() => setPickingFor(null)}
                          className="text-[11px] font-medium px-1.5 py-0.5 rounded-full text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors"
                          aria-label="Cancel"
                        >
                          <X className="w-3 h-3" aria-hidden="true" />
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => setPickingFor(poi.id)}
                        aria-label={`Add ${poi.name} to itinerary`}
                        className="mt-1.5 inline-flex items-center gap-1 text-[11px] font-semibold px-2.5 py-1 rounded-full bg-blue-50 text-blue-700 hover:bg-blue-100 transition-all cursor-pointer"
                      >
                        <Plus className="w-3 h-3" aria-hidden="true" />
                        Add to Itinerary
                        <ChevronDown className="w-3 h-3 ml-0.5" aria-hidden="true" />
                      </button>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
