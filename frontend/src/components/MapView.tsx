import { useMemo, useRef, useEffect } from 'react';
import { MapContainer, TileLayer, Marker, Popup, useMap } from 'react-leaflet';
import L from 'leaflet';
import type { POI, ItineraryItem } from '../types';

const CATEGORY_COLORS: Record<string, string> = {
  restaurant: '#EF4444',
  landmark:   '#3B82F6',
  hotel:      '#8B5CF6',
  attraction: '#F59E0B',
};

const CATEGORY_EMOJIS: Record<string, string> = {
  restaurant: '🍽',
  landmark:   '🏛',
  hotel:      '🏨',
  attraction: '🎡',
};

const LEGEND_ITEMS = [
  { label: 'Restaurants', color: CATEGORY_COLORS.restaurant, emoji: CATEGORY_EMOJIS.restaurant },
  { label: 'Landmarks',   color: CATEGORY_COLORS.landmark,   emoji: CATEGORY_EMOJIS.landmark   },
  { label: 'Hotels',      color: CATEGORY_COLORS.hotel,      emoji: CATEGORY_EMOJIS.hotel      },
  { label: 'Attractions', color: CATEGORY_COLORS.attraction, emoji: CATEGORY_EMOJIS.attraction },
];

function makePinIcon(color: string, label: string, category: string, highlighted = false) {
  const emoji     = CATEGORY_EMOJIS[category] ?? '📍';
  const safeLabel = label.replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  const w         = highlighted ? 52 : 44;
  const h         = highlighted ? 62 : 54;
  const cx        = w / 2;
  const scale     = w / 42;
  const borderClr = highlighted ? '#FCD34D' : 'white';
  const borderW   = highlighted ? 3 : 2.5;
  const fontSize  = highlighted ? 20 : 17;

  return L.divIcon({
    className: '',
    html: `
      <div
        role="img"
        aria-label="${safeLabel}"
        title="${safeLabel}"
        style="
          position:relative;
          width:${w}px;
          height:${h}px;
          filter:drop-shadow(0 4px 8px rgba(0,0,0,0.45));
          ${highlighted ? 'animation:pulse 1.2s ease-in-out infinite alternate;' : ''}
        "
      >
        <svg
          width="${w}" height="${h}"
          viewBox="0 0 42 50"
          xmlns="http://www.w3.org/2000/svg"
          style="position:absolute;top:0;left:0;"
        >
          <path
            d="M21 2C11.6 2 4 9.6 4 19C4 30.5 21 48 21 48C21 48 38 30.5 38 19C38 9.6 30.4 2 21 2Z"
            fill="${color}"
            stroke="${borderClr}"
            stroke-width="${borderW / scale}"
          />
          <circle cx="21" cy="19" r="9" fill="rgba(255,255,255,0.18)"/>
        </svg>
        <div style="
          position:absolute;
          top:0;left:0;
          width:${w}px;
          height:${w}px;
          display:flex;align-items:center;justify-content:center;
          font-size:${fontSize}px;
          line-height:1;
        ">${emoji}</div>
      </div>`,
    iconSize:    [w, h],
    iconAnchor:  [cx, h],
    popupAnchor: [0, -h],
  });
}

// ── Map controller: geocodes destination and keeps view synced with itinerary ──

interface MapControllerProps {
  itinerary: ItineraryItem[];
  destination: string;
}

function MapController({ itinerary, destination }: MapControllerProps) {
  const map = useMap();

  // Refs so geocode callback always reads the latest values without being deps
  const itineraryRef    = useRef(itinerary);
  itineraryRef.current  = itinerary;
  const geocodedCenter  = useRef<[number, number] | null>(null);
  const initialFitDone  = useRef(false);

  // Geocode the trip destination via Nominatim and fly there when itinerary is empty
  useEffect(() => {
    if (!destination) return;
    geocodedCenter.current = null;

    const controller = new AbortController();
    fetch(
      `https://nominatim.openstreetmap.org/search?q=${encodeURIComponent(destination)}&format=json&limit=1`,
      { signal: controller.signal, headers: { 'Accept-Language': 'en' } },
    )
      .then(r => r.json())
      .then((data: Array<{ lat: string; lon: string }>) => {
        if (data?.[0]) {
          const center: [number, number] = [parseFloat(data[0].lat), parseFloat(data[0].lon)];
          geocodedCenter.current = center;
          if (itineraryRef.current.length === 0) {
            map.flyTo(center, 12, { duration: 1.5 });
          }
        }
      })
      .catch(() => {});

    return () => controller.abort();
  }, [destination, map]);

  // Fit/fly the map to all itinerary markers whenever they change
  useEffect(() => {
    if (itinerary.length === 0) {
      // All items removed — return to the destination view if already geocoded
      if (geocodedCenter.current) {
        map.flyTo(geocodedCenter.current, 12, { duration: 1 });
      }
      return;
    }

    const points = itinerary.map(({ poi }) => [poi.lat, poi.lng] as [number, number]);
    const bounds = L.latLngBounds(points);

    if (!initialFitDone.current) {
      // First fit: no animation so the map snaps into place immediately on load
      map.fitBounds(bounds, { padding: [60, 60], maxZoom: 15 });
      initialFitDone.current = true;
    } else {
      // Subsequent changes: smooth fly animation
      map.flyToBounds(bounds, { padding: [60, 60], maxZoom: 15, duration: 1 });
    }
  }, [itinerary, map]);

  return null;
}

// ── MapView ────────────────────────────────────────────────────────────────────

interface MapViewProps {
  itinerary: ItineraryItem[];
  highlightPOI?: POI | null;
  destination: string;
}

export function MapView({ itinerary, highlightPOI, destination }: MapViewProps) {
  const markers = useMemo(() => {
    const seen = new Set<string>();
    const result: Array<{ poi: POI; highlighted: boolean }> = [];
    itinerary.forEach(item => {
      if (!seen.has(item.poi.id)) {
        seen.add(item.poi.id);
        result.push({ poi: item.poi, highlighted: false });
      }
    });
    if (highlightPOI && !seen.has(highlightPOI.id)) {
      result.push({ poi: highlightPOI, highlighted: true });
    }
    return result;
  }, [itinerary, highlightPOI]);

  // Compute the initial MapContainer center once on mount so there is no
  // world-view flash before the MapController effect fires.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const initialCenter = useMemo((): [number, number] => {
    if (itinerary.length > 0) {
      const lats = itinerary.map(i => i.poi.lat);
      const lngs = itinerary.map(i => i.poi.lng);
      return [
        (Math.min(...lats) + Math.max(...lats)) / 2,
        (Math.min(...lngs) + Math.max(...lngs)) / 2,
      ];
    }
    return [20, 0]; // neutral world center while geocoding resolves
  }, []); // intentionally empty — only used for the first render

  const initialZoom = itinerary.length > 0 ? 12 : 2;

  return (
    <div className="relative w-full h-full" aria-label="Interactive map of trip destinations">
      <style>{`@keyframes pulse{from{filter:drop-shadow(0 4px 8px rgba(0,0,0,.45))}to{filter:drop-shadow(0 4px 16px rgba(252,211,77,.9))}}`}</style>

      <MapContainer
        center={initialCenter}
        zoom={initialZoom}
        className="w-full h-full"
        zoomControl={true}
        attributionControl={true}
      >
        <TileLayer
          url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
          attribution='&copy; <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> contributors'
        />

        <MapController itinerary={itinerary} destination={destination} />

        {markers.map(({ poi, highlighted }) => (
          <Marker
            key={poi.id}
            position={[poi.lat, poi.lng]}
            icon={makePinIcon(CATEGORY_COLORS[poi.category], poi.name, poi.category, highlighted)}
          >
            <Popup minWidth={220} maxWidth={260}>
              <div className="text-sm overflow-hidden rounded-md">
                <img
                  src={poi.imageUrl}
                  alt={`Photo of ${poi.name}`}
                  className="w-full h-28 object-cover rounded-t-md block"
                  loading="lazy"
                />
                <div className="pt-2 pb-1 px-0.5">
                  <p className="font-semibold text-gray-900 text-sm leading-snug">{poi.name}</p>
                  <p className="text-gray-500 text-xs mt-0.5">{poi.subcategory}</p>
                  <p className="text-gray-400 text-xs mt-0.5 leading-snug">{poi.address}</p>
                </div>
              </div>
            </Popup>
          </Marker>
        ))}
      </MapContainer>

      {/* Legend — hidden on mobile/portrait, shown on desktop only */}
      <div
        className="hidden md:block absolute bottom-5 left-3 z-[1000] bg-white/95 backdrop-blur-sm rounded-xl shadow-lg px-4 py-3 border border-gray-100"
        aria-label="Map legend"
      >
        <p className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2.5">Legend</p>
        {LEGEND_ITEMS.map(({ label, color, emoji }) => (
          <div key={label} className="flex items-center gap-3 mb-2 last:mb-0">
            <span
              className="w-7 h-7 rounded-full flex-shrink-0 flex items-center justify-center text-sm"
              style={{ backgroundColor: color }}
              aria-hidden="true"
            >
              {emoji}
            </span>
            <span className="text-sm font-medium text-gray-700">{label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
